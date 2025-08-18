package codeexec

type Executor interface {
	Execute(code string) (string, error)
}
