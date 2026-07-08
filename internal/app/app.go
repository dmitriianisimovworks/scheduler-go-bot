package app

type App struct {
	httpServer HTTPServer
	telegram   TelegramRunner
}

func New() (*App, error) {
	return buildContainer()
}

func (a *App) Run() error {
	if a.httpServer != nil {
		go a.httpServer.Start()
	}

	if a.telegram != nil {
		return a.telegram.Run()
	}

	return nil
}
