package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/1F47E/rival/internal/server"
	"github.com/spf13/cobra"
)

var serverPort int

const serverHost = "127.0.0.1"

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the web dashboard",
	RunE:  serverAction,
}

func init() {
	serverCmd.Flags().IntVar(&serverPort, "port", 3333, "starting port (auto-increments if taken)")
	rootCmd.AddCommand(serverCmd)
}

func serverAction(cmd *cobra.Command, args []string) error {
	port, err := findPort(serverPort)
	if err != nil {
		return err
	}

	mux := server.New(Version)
	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", serverHost, port),
		Handler: mux,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	fmt.Printf("Dashboard: http://localhost:%d\n", port)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

func findPort(start int) (int, error) {
	for port := start; port <= start+10; port++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", serverHost, port))
		if err == nil {
			_ = ln.Close()
			return port, nil
		}
	}
	return 0, fmt.Errorf("no free port in range %d-%d", start, start+10)
}
