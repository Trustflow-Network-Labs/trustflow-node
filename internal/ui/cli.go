package ui

import (
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
)

type CLI struct{}

func (CLI) Print(msg string) {
	if strings.HasSuffix(msg, "\n") {
		fmt.Print(msg) // already has newline, avoid adding extra one
	} else {
		fmt.Println(msg) // add newline
	}
}

func (CLI) PromptConfirm(question string) bool {
	prompt := promptui.Prompt{
		Label:     question,
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil {
		return false
	}

	answer := strings.ToLower(strings.TrimSpace(result))
	return answer == "y" || answer == "yes"
}

func (CLI) Exit(code int) {
	os.Exit(code)
}
