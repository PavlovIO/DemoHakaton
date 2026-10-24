package handler

// pageParams разворачивает необязательные page/page_size списков. nil → 0: умолчания
// подставляет service.PageWindow, пределы из контракта проверяет validate.go.
func pageParams(page, pageSize *int) (int, int) {
	var p, s int
	if page != nil {
		p = *page
	}
	if pageSize != nil {
		s = *pageSize
	}
	return p, s
}
