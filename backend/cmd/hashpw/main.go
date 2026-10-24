// Команда hashpw — dev-утилита для сида (BE-16): печатает argon2id-хэш пароля в формате
// PHC, как его хранит users.password_hash.
//
//	echo -n 'Demo-Pass-2026' | go -C backend run ./cmd/hashpw
//
// Пароль читается из stdin, а не из аргумента: аргумент виден в истории shell и в ps.
// Завершающий перевод строки отбрасывается.
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/MaxPastukhov15/Liquidity_platform/backend/internal/service/auth"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "hashpw:", err)
		os.Exit(1)
	}
}

func run() error {
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	password := strings.TrimRight(line, "\r\n")
	if err := auth.ValidatePassword(password); err != nil {
		return err
	}
	hash, err := auth.HashPassword(context.Background(), password)
	if err != nil {
		return err
	}
	fmt.Println(hash)
	return nil
}
