package service

import "strings"

// likeEscaper экранирует метасимволы ILIKE; парный SQL — `ESCAPE '\'` (DECISIONS п. 40).
// Обратный слэш идёт первым: иначе он удвоил бы слэши, добавленные перед % и _.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// EscapeLike — пользовательский поиск q как подстрока, а не шаблон: без экранирования
// `a_b` нашёл бы `axb`. Вызывать перед передачей q в запрос с `ILIKE … ESCAPE '\'`.
func EscapeLike(q string) string {
	return likeEscaper.Replace(q)
}
