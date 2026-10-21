package handler

import "github.com/shopspring/decimal"

func init() {
	// Контракт: суммы и проценты — строки с масштабом из БД ("500000.00", "8.50").
	// shopspring/decimal по умолчанию срезает хвостовые нули при String()/MarshalJSON
	// ("500000", "8.5"); pgx отдаёт NUMERIC с масштабом в экспоненте, его и сохраняем.
	// Глобальная настройка пакета decimal — только здесь (проверяется money_test.go).
	decimal.TrimTrailingZeros = false
}
