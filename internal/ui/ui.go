package ui

type UI interface {
	Print(msg string)
	PromptConfirm(question string) bool
	Exit(code int)
}
