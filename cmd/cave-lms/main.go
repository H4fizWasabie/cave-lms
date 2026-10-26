package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/H4fizWasabie/cave-lms/internal/aiwriter"
	"github.com/H4fizWasabie/cave-lms/internal/app"
	"github.com/H4fizWasabie/cave-lms/internal/config"
	"github.com/H4fizWasabie/cave-lms/internal/mailer"
)

func main() {
	if err := run(os.Args[1:], os.Stdin); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

// run starts the server with no arguments, or runs a command-line step such as create-admin.
func run(args []string, stdin io.Reader) error {
	if len(args) == 0 {
		return serve()
	}
	switch args[0] {
	case "create-admin":
		return createAdmin(args[1:], stdin)
	default:
		return fmt.Errorf("unknown command %q (commands: create-admin)", args[0])
	}
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	var m mailer.Mailer = mailer.Unconfigured{}
	if cfg.ResendAPIKey != "" {
		m = mailer.Resend{APIKey: cfg.ResendAPIKey, From: cfg.EmailFrom}
		log.Printf("Sending email through Resend as %s", cfg.EmailFrom)
	} else {
		log.Print("RESEND_API_KEY is not set: emails will not be sent")
	}
	w := aiwriter.New(cfg.OpenRouterAPIKey, cfg.OpenRouterModel, cfg.OpenRouterWebSearch)
	if _, off := w.(aiwriter.Unconfigured); off {
		log.Print("OPENROUTER_API_KEY or OPENROUTER_MODEL is not set: the AI will not write Exam questions or Lesson text or mark Written answers")
	} else {
		log.Printf("Writing Exam questions and Lesson text and marking Written answers with %s through OpenRouter (web search %t)", cfg.OpenRouterModel, cfg.OpenRouterWebSearch)
	}
	if cfg.BCLWebhookKey != "" && cfg.BCLAPIToken == "" {
		log.Print("BCL_API_TOKEN is not set: BCL webhook orders are not confirmed with BCL, so a forged one would count")
	}
	application, err := app.New(context.Background(), cfg, m, w)
	if err != nil {
		return err
	}
	defer application.Close()

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           application,
		ReadHeaderTimeout: 5 * time.Second,
	}
	application.Started(cfg.ListenAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// createAdmin makes the Admin account. The password comes from CAVE_ADMIN_PASSWORD or the first line of stdin, never a flag.
func createAdmin(args []string, stdin io.Reader) error {
	flags := flag.NewFlagSet("create-admin", flag.ContinueOnError)
	name := flags.String("name", "", "the Admin's name")
	email := flags.String("email", "", "the Admin's email address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	password, err := adminPassword(stdin)
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	application, err := app.New(ctx, cfg, mailer.Unconfigured{}, aiwriter.Unconfigured{})
	if err != nil {
		return err
	}
	defer application.Close()
	if err := application.CreateAdmin(ctx, *name, *email, password); err != nil {
		return err
	}
	log.Printf("Admin account ready for %s", *email)
	return nil
}

func adminPassword(stdin io.Reader) (string, error) {
	if password, ok := os.LookupEnv("CAVE_ADMIN_PASSWORD"); ok {
		return password, nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
