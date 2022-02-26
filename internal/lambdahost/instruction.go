package lambdahost

type instruction string

const (
	instructionShutdown instruction = "shutdown"
	instructionRestart              = "restart"
)
