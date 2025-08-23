package main

import (
	"log"
	"time"

	"github.com/joho/godotenv"
	"gopkg.in/telebot.v3"

	"github.com/yourname/telegram-schedule-bot/internal/bot"
	"github.com/yourname/telegram-schedule-bot/internal/config"
	"github.com/yourname/telegram-schedule-bot/internal/scheduler"
	"github.com/yourname/telegram-schedule-bot/internal/store"
)

func main() {
	_ = godotenv.Load()
	cfg := config.Load()

	st, err := store.Open(cfg.DatabaseURL)
	if err != nil { log.Fatal(err) }

	pref := telebot.Settings{
		Token: cfg.BotToken,
		Poller: &telebot.LongPoller{Timeout: 10 * time.Second},
	}
	b, err := telebot.NewBot(pref)
	if err != nil { log.Fatal(err) }

	sch := scheduler.New(b, st)
	app := bot.New(b, st, sch)
	app.SetupHandlers(cfg.DefaultTZ)

	if err := sch.RescheduleEnabledUsers(); err != nil { log.Println("reschedule:", err) }

	b.Start()
}
