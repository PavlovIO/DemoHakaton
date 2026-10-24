package service

import "math"

// Пагинация по умолчанию — как в контракте (Page, PageSize).
const (
	defaultPage     = 1
	defaultPageSize = 20
)

// PageWindow — страница и её размер с умолчаниями контракта и LIMIT/OFFSET для SQL.
// У page в контракте нет maximum: смещение за пределом int32 упирается в MaxInt32 —
// это страница за концом списка (пусто, total честный), а не отрицательный OFFSET (500)
// или переполнение в чужую страницу.
func PageWindow(page, pageSize int) (outPage, outSize int, limit, offset int32) {
	if page < 1 {
		page = defaultPage
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	limit, offset = math.MaxInt32, math.MaxInt32
	if pageSize < math.MaxInt32 {
		limit = int32(pageSize)
	}
	if page-1 <= math.MaxInt32/pageSize {
		offset = int32((page - 1) * pageSize)
	}
	return page, pageSize, limit, offset
}
