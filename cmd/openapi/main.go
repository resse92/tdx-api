package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/resse/tdx-api/internal/httpapi"
)

func main() {
	out := flag.String("out", "docs/openapi.json", "输出文件")
	flag.Parse()
	data, err := httpapi.OpenAPIJSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	data = append(data, '\n')
	if err := os.WriteFile(*out, data, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
