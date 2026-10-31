package main

import (
	"fmt"

	"agentcollab.local/agentcollab/internal/buildinfo"
)

func main() {
	fmt.Println(buildinfo.Current("agentcollabctl"))
}
