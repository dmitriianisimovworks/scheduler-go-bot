// Command oauth-setup is a one-time local tool to obtain a Google OAuth
// refresh token for the calendar-organizer account. Run it once locally,
// approve access in the browser, and copy the printed refresh token into
// the bot's .env as GOOGLE_OAUTH_REFRESH_TOKEN.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
)

func main() {
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <path-to-client-secret.json>", os.Args[0])
	}

	credBytes, err := os.ReadFile(os.Args[1])
	if err != nil {
		log.Fatalf("read client secret file: %v", err)
	}

	config, err := google.ConfigFromJSON(credBytes, calendar.CalendarScope)
	if err != nil {
		log.Fatalf("parse client secret file: %v", err)
	}
	config.RedirectURL = "http://localhost:8085/callback"

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	server := &http.Server{Addr: "localhost:8085"}
	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.RawQuery)
			fmt.Fprintln(w, "Authorization failed, check the terminal.")
			return
		}
		codeCh <- code
		fmt.Fprintln(w, "Authorization complete, you can close this tab.")
	})

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
	fmt.Println("Open this URL in your browser and approve access:")
	fmt.Println(authURL)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		log.Fatalf("callback error: %v", err)
	}

	_ = server.Shutdown(context.Background())

	token, err := config.Exchange(context.Background(), code)
	if err != nil {
		log.Fatalf("exchange code for token: %v", err)
	}

	if token.RefreshToken == "" {
		log.Fatal("no refresh token returned; revoke prior access at https://myaccount.google.com/permissions and re-run (approval_prompt=force should prevent this, but Google sometimes omits it on repeat consent)")
	}

	fmt.Println("\nSuccess. Add these to the bot's .env:")
	fmt.Printf("GOOGLE_OAUTH_CLIENT_ID=%s\n", config.ClientID)
	fmt.Printf("GOOGLE_OAUTH_CLIENT_SECRET=%s\n", config.ClientSecret)
	fmt.Printf("GOOGLE_OAUTH_REFRESH_TOKEN=%s\n", token.RefreshToken)
}
