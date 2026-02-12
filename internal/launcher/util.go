package launcher

import "strings"

func joinArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, a := range args {
		if strings.ContainsAny(a, " \t") {
			quoted[i] = "'" + a + "'"
		} else {
			quoted[i] = a
		}
	}
	return strings.Join(quoted, " ")
}
