package notes

import (
	"errors"
	"strings"
)

type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
}

func Validate(title, content string) error {
	if strings.TrimSpace(title) == "" || len(title) > 256 || strings.TrimSpace(content) == "" || len(content) > 8192 {
		return errors.New("invalid note")
	}
	return nil
}
