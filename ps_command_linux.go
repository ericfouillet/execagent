// +build linux

package execagent

func getPsCommand(procName string) (string, []string) {
	return "ps", []string{"-C", procName, "-o", "pid="}
}
