-- Следы KYC демо-компаний (BE-16) по их итоговым статусам: применяется последним, после
-- seed_demo_common.sql и файла варианта — вариант меняет статусы «Альфы» и Beta. Пишет то
-- же, что submitKyc (BE-05, BE-13) и решение админа (BE-12): status_history, audit_log и
-- уведомления. Компания без kyc_submitted_at (draft) следов не получает.
--
-- Идемпотентно: NOT EXISTS по компании и переходу / действию / типу уведомления.
-- Уведомления старше 30 дней прочитаны: непрочитанными в демо остаются только свежие.

-- Подавшие KYC демо-компании, их пользователь и решение: verified — verified_at,
-- rejected — через день после подачи (как у флага «Омеги» в common).
CREATE OR REPLACE TEMP VIEW demo_kyc AS
SELECT o.id AS org_id, o.name, u.id AS user_id, u.role AS user_role,
       o.kyc_submitted_at AS submitted_at, o.kyc_reject_reason AS reject_reason,
       CASE WHEN o.kyc_status IN ('verified', 'rejected') THEN o.kyc_status::text END AS decision,
       CASE o.kyc_status WHEN 'verified' THEN o.verified_at
                         WHEN 'rejected' THEN o.kyc_submitted_at + interval '1 day' END AS decided_at,
       (SELECT count(*) FROM aml_flags f WHERE f.organization_id = o.id) AS flags
FROM organizations o
JOIN users u ON u.organization_id = o.id
WHERE u.email LIKE '%@liquidity.local' AND o.kyc_submitted_at IS NOT NULL;

-- 1. История: подача — пользователь компании, решение — admin; у отказа причина в comment.
INSERT INTO status_history (entity_type, entity_id, from_status, to_status, actor_user_id, actor_role, comment, created_at)
SELECT 'organization', k.org_id, v.from_status, v.to_status, v.actor_id, v.actor_role, v.comment, v.created_at
FROM demo_kyc k
JOIN LATERAL (VALUES
    ('draft', 'under_review', k.user_id, k.user_role, NULL, k.submitted_at),
    ('under_review', k.decision, 'aaaaaaaa-0000-0000-0000-000000000001'::uuid, 'admin'::user_role,
     CASE WHEN k.decision = 'rejected' THEN k.reject_reason END, k.decided_at)
) AS v(from_status, to_status, actor_id, actor_role, comment, created_at) ON v.to_status IS NOT NULL
WHERE NOT EXISTS (SELECT 1 FROM status_history h
                  WHERE h.entity_type = 'organization' AND h.entity_id = k.org_id AND h.to_status = v.to_status);

-- 2. Аудит: kyc.submit от компании, kyc.approve / kyc.reject от admin (причина — в meta).
INSERT INTO audit_log (user_id, action, entity_type, entity_id, meta, created_at)
SELECT v.user_id, v.action, 'organization', k.org_id::text, v.meta, v.created_at
FROM demo_kyc k
JOIN LATERAL (VALUES
    ('kyc.submit', k.user_id, '{}'::jsonb, k.submitted_at),
    (CASE k.decision WHEN 'verified' THEN 'kyc.approve' WHEN 'rejected' THEN 'kyc.reject' END,
     'aaaaaaaa-0000-0000-0000-000000000001'::uuid,
     CASE WHEN k.decision = 'rejected' THEN jsonb_build_object('reason', k.reject_reason) ELSE '{}'::jsonb END,
     k.decided_at)
) AS v(action, user_id, meta, created_at) ON v.action IS NOT NULL
WHERE NOT EXISTS (SELECT 1 FROM audit_log a WHERE a.action = v.action AND a.entity_id = k.org_id::text);

-- 3. Уведомления: админу — о подаче и (если скрининг что-то нашёл) одно aml_flag_created
-- на подачу (DECISIONS п. 14); компании — о решении. Заголовки — как у сервисов.
INSERT INTO notifications (user_id, type, title, entity_type, entity_id, is_read, created_at)
SELECT v.user_id, v.type::notification_type, v.title, 'organization', k.org_id::text,
       v.created_at < now() - interval '30 days', v.created_at
FROM demo_kyc k
JOIN LATERAL (VALUES
    ('aaaaaaaa-0000-0000-0000-000000000001'::uuid, 'kyc_submitted',
     'Компания отправлена на проверку: ' || k.name, k.submitted_at),
    ('aaaaaaaa-0000-0000-0000-000000000001'::uuid, CASE WHEN k.flags > 0 THEN 'aml_flag_created' END,
     'AML: ' || k.flags || ' новых флагов — ' || k.name, k.submitted_at),
    (k.user_id, CASE k.decision WHEN 'verified' THEN 'kyc_approved' WHEN 'rejected' THEN 'kyc_rejected' END,
     CASE k.decision WHEN 'verified' THEN 'Компания подтверждена'
                     ELSE 'Проверка компании не пройдена: ' || k.reject_reason END, k.decided_at)
) AS v(user_id, type, title, created_at) ON v.type IS NOT NULL
WHERE NOT EXISTS (SELECT 1 FROM notifications n
                  WHERE n.user_id = v.user_id AND n.type = v.type::notification_type AND n.entity_id = k.org_id::text);
