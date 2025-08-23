package scheduler

import (
	"fmt"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jmoiron/sqlx"
	"gopkg.in/telebot.v3"

	"github.com/okpulse/telegram-schedule-bot/internal/store"
	"github.com/okpulse/telegram-schedule-bot/internal/timeutil"
)

type Scheduler struct {
	S   gocron.Scheduler
	Bot *telebot.Bot
	DB  *sqlx.DB
	St  *store.Store
}

func New(bot *telebot.Bot, st *store.Store) *Scheduler {
	s, err := gocron.NewScheduler(gocron.WithLocation(time.UTC))
	if err != nil {
		panic(err)
	}
	s.Start()
	return &Scheduler{S: s, Bot: bot, DB: st.DB, St: st}
}

func (sc *Scheduler) userTag(userID int64) string { return fmt.Sprintf("user:%d", userID) }

func (sc *Scheduler) ClearUser(userID int64) {
	sc.S.RemoveByTags(sc.userTag(userID))
}

func (sc *Scheduler) ScheduleAllForUser(u store.User) error {
	sc.ClearUser(u.ID)
	tasks, err := sc.St.GetTasksForUser(u.ID)
	if err != nil {
		return err
	}

	loc, err := time.LoadLocation(u.TZ)
	if err != nil {
		return err
	}
	now := time.Now().In(loc)
	weekdayBit := timeutil.WeekdayBit(now.Weekday())

	for _, t := range tasks {
		if (t.DaysMask & weekdayBit) == 0 {
			continue
		}
		startLocal, _ := timeutil.LocalDateTime(u.TZ, t.StartH, t.StartM, 0)
		endLocal, _ := timeutil.LocalDateTime(u.TZ, t.EndH, t.EndM, 0)
		if endLocal.Before(startLocal) {
			continue
		}

		if startLocal.After(now) {
			startUTC := startLocal.UTC()
			_, _ = sc.S.NewJob(
				gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(startUTC)),
				gocron.NewTask(func(chatID int64, userID, taskID int64, title string) {
					_ = sc.St.StartRun(userID, taskID, time.Now().UTC())
					sc.Bot.Send(&telebot.Chat{ID: chatID}, "🔔Старт задачи: "+title)
				}, u.TGID, u.ID, t.ID, t.Title),
				gocron.WithTags(sc.userTag(u.ID)),
			)
		}
		if endLocal.After(now) {
			endUTC := endLocal.UTC()
			_, _ = sc.S.NewJob(
				gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(endUTC)),
				gocron.NewTask(func(chatID int64, userID, taskID int64, title string) {
					_ = sc.St.EndRun(userID, taskID, time.Now().UTC())
					sc.Bot.Send(&telebot.Chat{ID: chatID}, "✅Финиш задачи: "+title)
				}, u.TGID, u.ID, t.ID, t.Title),
				gocron.WithTags(sc.userTag(u.ID)),
			)
		}
	}

	if next, err := timeutil.NextLocalMidnightPlus(u.TZ, 5); err == nil {
		_, _ = sc.S.NewJob(
			gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(next)),
			gocron.NewTask(func(userTGID int64) {
				usr, err := sc.St.GetUserByTGID(userTGID)
				if err == nil && usr.ControlEnabled {
					_ = sc.ScheduleAllForUser(usr)
				}
			}, u.TGID),
			gocron.WithTags(sc.userTag(u.ID)),
		)
	}
	return nil
}

func (sc *Scheduler) RescheduleEnabledUsers() error {
	users, err := sc.St.UsersWithControlEnabled()
	if err != nil {
		return err
	}
	for _, u := range users {
		if err := sc.ScheduleAllForUser(u); err != nil {
			return err
		}
	}
	return nil
}
