package ui

import (
	"fmt"
	"os"

	"github.com/manifoldco/promptui"
)

type CLI struct{}

func (CLI) Print(msg string) {
	fmt.Println(msg)
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
	return result == "y" || result == "yes"
}

func (CLI) Exit(code int) {
	os.Exit(code)
}
