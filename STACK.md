# Meeting Bot Stack

1. Goal: Telegram tool for company meeting scheduling.
2. Scope: MVP for test assignment, no frontend or mini app.
3. Deployment: VPS for 3-4 days during review.
4. Runtime: Docker Compose.
5. App language: Go.
6. HTTP router: chi.
7. Telegram library: telebot.
8. Telegram delivery mode: webhook.
9. Public entrypoint: domain subdomain, for example `bot.example.com`.
10. Reverse proxy: Nginx.
11. TLS: Let's Encrypt certificate for webhook HTTPS.
12. Database: Postgres.
13. Postgres role: source of truth for users, meetings, participants.
14. Google Sheets role: readable schedule/export view for reviewers.
15. Google auth: service account credentials.
16. Redis: not included in MVP.
17. Frontend: not included in MVP.
18. Bot commands: `/start`, `/new`, `/day`, `/week`, `/cancel`, `/help`.
19. Core rule: meeting cannot be created if any participant has overlap.
20. Timezone: `Europe/Moscow`.
21. API endpoints: `POST /telegram/webhook`, `GET /healthz`.
22. Webhook protection: Telegram secret token.
23. Config: environment variables via `.env`.
24. Migrations: SQL migrations for Postgres schema.
25. Logs: container stdout/stderr.
26. Backups: optional, not required for short-lived test deployment.
27. Main entities: users, meetings, meeting_participants.
28. Success criteria: reviewers can add meetings and view day/week schedule.
29. Delivery artifact: Telegram bot link, Google Sheet link, short README.
30. Optional future extension: small internal web app for calendar browsing/admin.
31. Web app is not part of MVP and would be considered only after the bot flow is stable.
32. Later: discuss architecture, data model, and conversation flow.
