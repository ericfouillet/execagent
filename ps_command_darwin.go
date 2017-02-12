package execagent

func getPsCommand(procName string) (string, []string) {
	return "pgrep", []string{procName}
}
