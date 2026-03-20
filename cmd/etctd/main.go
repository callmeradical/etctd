package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"etctd/internal/app"
	storeetcd "etctd/internal/store/etcd"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	st, err := storeetcd.Open(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	defer st.Close()

	a := app.New(st, os.Stdout, os.Stderr)
	if err := a.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
