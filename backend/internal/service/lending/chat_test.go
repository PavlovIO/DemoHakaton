package lending_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/apperr"
	repository "github.com/MaxPastukhov15/Liquidity_platform/backend/internal/repository/sqlc"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/lending"
	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/servicetest"
)

// fakeLimiter — RateLimiter с управляемым ответом; запоминает ключи вызовов Allow.
type fakeLimiter struct {
	denied     bool
	retryAfter time.Duration
	keys       []string
}

func (l *fakeLimiter) Allow(key string, _ int, _ time.Duration) (time.Duration, bool) {
	l.keys = append(l.keys, key)
	if l.denied {
		return l.retryAfter, false
	}
	return 0, true
}

func (l *fakeLimiter) Reset(string) {}

// Чатные запросы fakeAppStore: видимость ListMessagesVisible — та же, что у GetOfferVisible.

func (s *fakeAppStore) CreateMessage(_ context.Context, arg repository.CreateMessageParams) (repository.Message, error) {
	s.calls = append(s.calls, "CreateMessage")
	m := repository.Message{ID: int64(len(s.messages) + 1), OfferID: arg.OfferID, AuthorUserID: arg.AuthorUserID, Text: arg.Text}
	s.messages = append(s.messages, m)
	return m, nil
}

func (s *fakeAppStore) LockOfferChat(context.Context, uuid.UUID) error {
	s.calls = append(s.calls, "LockOfferChat")
	return nil
}

func (s *fakeAppStore) ListMessagesVisible(_ context.Context, arg repository.ListMessagesVisibleParams) ([]repository.ListMessagesVisibleRow, error) {
	s.calls = append(s.calls, "ListMessagesVisible")
	rows := []repository.ListMessagesVisibleRow{}
	for _, m := range s.messages {
		if m.OfferID != arg.OfferID || m.ID <= arg.AfterID || len(rows) == int(arg.MaxRows) || !s.messageVisible(m, arg) {
			continue
		}
		author := s.authors[m.AuthorUserID]
		org := s.orgs[*author.OrgID]
		rows = append(rows, repository.ListMessagesVisibleRow{
			Message: m, AuthorRole: author.ViewerRole(),
			AuthorOrgID: org.ID, AuthorOrgName: org.Name, AuthorOrgCountryCode: org.CountryCode,
		})
	}
	return rows, nil
}

func (s *fakeAppStore) messageVisible(m repository.Message, arg repository.ListMessagesVisibleParams) bool {
	for _, o := range s.offers {
		if o.ID == m.OfferID {
			return s.offerVisible(o, arg.ViewerRole, arg.ViewerOrg)
		}
	}
	return false
}

// party — компания и её пользователь, зарегистрированный как возможный автор сообщений.
func (s *fakeAppStore) party(role repository.UserRole) service.Principal {
	p := s.org(role)
	if s.authors == nil {
		s.authors = map[uuid.UUID]service.Principal{}
	}
	s.authors[p.UserID] = p
	return p
}

// chatOffer — заёмщик, кредитор и оффер кредитора в статусе status по заявке заёмщика.
func chatOffer(s *fakeAppStore, status repository.OfferStatus) (borrower, lender service.Principal, offerID uuid.UUID) {
	borrower, lender = s.party(repository.UserRoleBorrower), s.party(repository.UserRoleLender)
	offerID = s.offer(s.app(borrower, repository.ApplicationStatusPublished), lender, status)
	return borrower, lender, offerID
}

func newChat(s *fakeAppStore, l service.RateLimiter) *lending.Chat {
	return lending.NewChat(lending.ChatDeps{Store: s, Tx: fakeAppTx{s}, Limiter: l})
}

func TestChatSend(t *testing.T) {
	ctx := context.Background()
	admin := service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}

	t.Run("admin — FORBIDDEN, лимитер и стор не вызваны", func(t *testing.T) {
		s := newFakeAppStore()
		appID := s.app(s.org(repository.UserRoleBorrower), repository.ApplicationStatusPublished)
		offerID := s.offer(appID, s.org(repository.UserRoleLender), repository.OfferStatusSent)
		l := &fakeLimiter{}
		_, err := newChat(s, l).Send(ctx, admin, offerID, "привет")
		servicetest.WantCode(t, err, apperr.CodeForbidden)
		if len(l.keys) != 0 || len(s.calls) != 0 {
			t.Fatalf("limiter = %v, store calls = %v", l.keys, s.calls)
		}
	})

	t.Run("лимит исчерпан — RATE_LIMITED с RetryAfter, сообщение не создано", func(t *testing.T) {
		s := newFakeAppStore()
		borrower := s.org(repository.UserRoleBorrower)
		offerID := s.offer(s.app(borrower, repository.ApplicationStatusPublished), s.org(repository.UserRoleLender), repository.OfferStatusSent)
		l := &fakeLimiter{denied: true, retryAfter: 12 * time.Second}
		_, err := newChat(s, l).Send(ctx, borrower, offerID, "привет")
		servicetest.WantCode(t, err, apperr.CodeRateLimited)
		if e, _ := apperr.As(err); e.RetryAfter != 12*time.Second {
			t.Fatalf("RetryAfter = %v, want 12s", e.RetryAfter)
		}
		if len(l.keys) != 1 || l.keys[0] != "chat:send:"+borrower.UserID.String() {
			t.Fatalf("limiter keys = %v", l.keys)
		}
		if len(s.messages) != 0 || slices.Contains(s.calls, "LockOfferChat") {
			t.Fatalf("messages = %v, calls = %v", s.messages, s.calls)
		}
	})

	t.Run("lender, оффер sent, текст из пробелов — сохранён как есть, уведомление заёмщику", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, lender, offerID := chatOffer(s, repository.OfferStatusSent)
		got, err := newChat(s, &fakeLimiter{}).Send(ctx, lender, offerID, "   ")
		if err != nil {
			t.Fatal(err)
		}
		if len(s.messages) != 1 || s.messages[0].Text != "   " || s.messages[0].AuthorUserID != lender.UserID {
			t.Fatalf("messages = %+v", s.messages)
		}
		if got.ID != s.messages[0].ID || got.OfferID != offerID || got.Text != "   " || !got.IsMine ||
			got.AuthorUserID != lender.UserID || got.AuthorRole != repository.UserRoleLender ||
			got.AuthorOrg.ID != *lender.OrgID || got.AuthorOrg.Name != "ООО lender" || got.AuthorOrg.CountryCode != "RU" {
			t.Fatalf("got %+v", got)
		}
		if len(s.orgNotices) != 1 || s.orgNotices[0].OrganizationID != *borrower.OrgID ||
			s.orgNotices[0].Type != repository.NotificationTypeMessageReceived ||
			s.orgNotices[0].EntityType != repository.EntityTypeOffer || s.orgNotices[0].EntityID != offerID.String() {
			t.Fatalf("notices = %+v", s.orgNotices)
		}
		// Блокировка чата оффера — сразу перед вставкой: id сообщений одного оффера
		// коммитятся по порядку, и polling по id > after_id ничего не пропускает.
		i := slices.Index(s.calls, "CreateMessage")
		if i < 1 || s.calls[i-1] != "LockOfferChat" {
			t.Fatalf("calls = %v, want LockOfferChat перед CreateMessage", s.calls)
		}
	})

	t.Run("borrower, два сообщения — два уведомления кредитору", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, lender, offerID := chatOffer(s, repository.OfferStatusSent)
		chat := newChat(s, &fakeLimiter{})
		for _, text := range []string{"первое", "второе"} {
			if _, err := chat.Send(ctx, borrower, offerID, text); err != nil {
				t.Fatal(err)
			}
		}
		if len(s.orgNotices) != 2 {
			t.Fatalf("notices = %+v, want 2", s.orgNotices)
		}
		for _, n := range s.orgNotices {
			if n.OrganizationID != *lender.OrgID || n.Type != repository.NotificationTypeMessageReceived {
				t.Fatalf("notice = %+v", n)
			}
		}
	})

	t.Run("чужой кредитор — NOT_FOUND, сообщение не создано", func(t *testing.T) {
		s := newFakeAppStore()
		_, _, offerID := chatOffer(s, repository.OfferStatusSent)
		l := &fakeLimiter{}
		_, err := newChat(s, l).Send(ctx, s.party(repository.UserRoleLender), offerID, "привет")
		servicetest.WantCode(t, err, apperr.CodeNotFound)
		if len(s.messages) != 0 || len(l.keys) != 0 {
			t.Fatalf("messages = %+v, limiter = %v", s.messages, l.keys)
		}
	})

	t.Run("21-е сообщение за минуту — RATE_LIMITED, сообщение не создано", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, _, offerID := chatOffer(s, repository.OfferStatusSent)
		now := testNow
		limiter := service.NewMemoryRateLimiterWithClock(func() time.Time { return now })
		chat := newChat(s, limiter)
		for i := range 20 {
			if _, err := chat.Send(ctx, borrower, offerID, "сообщение"); err != nil {
				t.Fatalf("сообщение %d: %v", i+1, err)
			}
			now = now.Add(time.Second)
		}
		_, err := chat.Send(ctx, borrower, offerID, "лишнее")
		servicetest.WantCode(t, err, apperr.CodeRateLimited)
		if e, _ := apperr.As(err); e.RetryAfter != 40*time.Second {
			t.Fatalf("RetryAfter = %v, want 40s (первое сообщение выпадет из окна)", e.RetryAfter)
		}
		if len(s.messages) != 20 {
			t.Fatalf("messages = %d, want 20", len(s.messages))
		}
	})

	for _, tc := range []struct {
		status repository.OfferStatus
		open   bool
	}{
		{repository.OfferStatusSent, true},
		{repository.OfferStatusAccepted, true},
		{repository.OfferStatusRejected, false},
		{repository.OfferStatusWithdrawn, false},
	} {
		t.Run("оффер "+string(tc.status), func(t *testing.T) {
			s := newFakeAppStore()
			borrower, _, offerID := chatOffer(s, tc.status)
			l := &fakeLimiter{}
			_, err := newChat(s, l).Send(ctx, borrower, offerID, "привет")
			if tc.open {
				if err != nil || len(s.messages) != 1 {
					t.Fatalf("err = %v, messages = %+v", err, s.messages)
				}
				return
			}
			servicetest.WantCode(t, err, apperr.CodeInvalidTransition)
			// Закрытый чат лимит не расходует: иначе повторы отправки в отозванный оффер
			// дали бы 429 во всех чатах пользователя.
			if len(s.messages) != 0 || len(s.orgNotices) != 0 || len(l.keys) != 0 {
				t.Fatalf("messages = %+v, notices = %+v, limiter = %v", s.messages, s.orgNotices, l.keys)
			}
		})
	}
}

func (s *fakeAppStore) MarkMessagesRead(_ context.Context, arg repository.MarkMessagesReadParams) error {
	s.calls = append(s.calls, "MarkMessagesRead")
	s.readMarks = append(s.readMarks, arg)
	return nil
}

// message — сообщение автора с заданным id (курсор polling) в памяти фейка.
func (s *fakeAppStore) message(id int64, offerID uuid.UUID, author service.Principal) {
	s.messages = append(s.messages, repository.Message{ID: id, OfferID: offerID, AuthorUserID: author.UserID, Text: "текст"})
}

func TestChatList(t *testing.T) {
	ctx := context.Background()

	t.Run("lender читает 3 сообщения — указатель на последнем, is_mine только у своих", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, lender, offerID := chatOffer(s, repository.OfferStatusSent)
		s.message(5, offerID, borrower)
		s.message(6, offerID, lender)
		s.message(7, offerID, borrower)
		got, err := newChat(s, &fakeLimiter{}).List(ctx, lender, offerID, 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 3 || got[0].ID != 5 || got[2].ID != 7 {
			t.Fatalf("got %+v", got)
		}
		if got[0].IsMine || !got[1].IsMine || got[2].IsMine {
			t.Fatalf("is_mine = %v %v %v, want false true false", got[0].IsMine, got[1].IsMine, got[2].IsMine)
		}
		if got[0].AuthorRole != repository.UserRoleBorrower || got[0].AuthorOrg.ID != *borrower.OrgID {
			t.Fatalf("author = %s %+v", got[0].AuthorRole, got[0].AuthorOrg)
		}
		want := repository.MarkMessagesReadParams{OfferID: offerID, UserID: lender.UserID, LastReadMessageID: 7}
		if len(s.readMarks) != 1 || s.readMarks[0] != want {
			t.Fatalf("read marks = %+v, want %+v", s.readMarks, want)
		}
	})

	t.Run("admin читает — 200, указатель не пишется", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, _, offerID := chatOffer(s, repository.OfferStatusSent)
		s.message(1, offerID, borrower)
		admin := service.Principal{UserID: uuid.New(), Role: string(repository.UserRoleAdmin)}
		got, err := newChat(s, &fakeLimiter{}).List(ctx, admin, offerID, 0, 50)
		if err != nil || len(got) != 1 || got[0].IsMine {
			t.Fatalf("got %+v, err = %v", got, err)
		}
		if len(s.readMarks) != 0 {
			t.Fatalf("read marks = %+v", s.readMarks)
		}
	})

	t.Run("нет новых — пустой срез, не nil, указатель не пишется", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, _, offerID := chatOffer(s, repository.OfferStatusSent)
		s.message(1, offerID, borrower)
		got, err := newChat(s, &fakeLimiter{}).List(ctx, borrower, offerID, 1, 50)
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("got %#v, err = %v", got, err)
		}
		if len(s.readMarks) != 0 {
			t.Fatalf("read marks = %+v", s.readMarks)
		}
	})

	t.Run("polling: after_id второго — только третье", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, lender, offerID := chatOffer(s, repository.OfferStatusSent)
		for id := int64(1); id <= 3; id++ {
			s.message(id, offerID, borrower)
		}
		got, err := newChat(s, &fakeLimiter{}).List(ctx, lender, offerID, 2, 50)
		if err != nil || len(got) != 1 || got[0].ID != 3 {
			t.Fatalf("got %+v, err = %v", got, err)
		}
	})

	t.Run("чужой заёмщик — NOT_FOUND, указатель не пишется", func(t *testing.T) {
		s := newFakeAppStore()
		borrower, _, offerID := chatOffer(s, repository.OfferStatusSent)
		s.message(1, offerID, borrower)
		_, err := newChat(s, &fakeLimiter{}).List(ctx, s.party(repository.UserRoleBorrower), offerID, 0, 50)
		servicetest.WantCode(t, err, apperr.CodeNotFound)
		if len(s.readMarks) != 0 {
			t.Fatalf("read marks = %+v", s.readMarks)
		}
	})
}
