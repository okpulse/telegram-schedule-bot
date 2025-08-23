package bot

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/telebot.v3"

	"github.com/yourname/telegram-schedule-bot/internal/scheduler"
	"github.com/yourname/telegram-schedule-bot/internal/store"
	"github.com/yourname/telegram-schedule-bot/internal/timeutil"
)

const chooseDaysTitle = "Выберите дни (нажимайте, затем 'Готово')"

type BotApp struct {
	Bot *telebot.Bot
	St  *store.Store
	Sch *scheduler.Scheduler

	addMu    sync.Mutex
	addState map[int64]*AddState

	// repeat menu
	repMK       *telebot.ReplyMarkup
	btnRepToday telebot.Btn
	btnRepDaily telebot.Btn
	btnRepWork  telebot.Btn
	btnRepCust  telebot.Btn

	// custom days (use static uniques; keyboard re-rendered with labels)
	btnMon      telebot.Btn
	btnTue      telebot.Btn
	btnWed      telebot.Btn
	btnThu      telebot.Btn
	btnFri      telebot.Btn
	btnSat      telebot.Btn
	btnSun      telebot.Btn
	btnDaysDone telebot.Btn

	// report menu
	reportMK    *telebot.ReplyMarkup
	btnRptDay   telebot.Btn
	btnRptWeek  telebot.Btn
	btnRptMonth telebot.Btn
	btnRptAll   telebot.Btn

	// per-task list buttons
	btnTaskToggle telebot.Btn
	btnTaskDelete telebot.Btn
}

type AddState struct {
	Step     int
	Title    string
	StartH   int
	StartM   int
	EndH     int
	EndM     int
	Repeat   string
	DaysMask int
}

func New(b *telebot.Bot, st *store.Store, sch *scheduler.Scheduler) *BotApp {
	return &BotApp{Bot: b, St: st, Sch: sch, addState: make(map[int64]*AddState)}
}

func (a *BotApp) SetupHandlers(defaultTZ string) {
	// main reply keyboard
	rp := &telebot.ReplyMarkup{}
	btnAdd := rp.Text("➕ Добавить задачу")
	btnList := rp.Text("📋 Список задач")
	btnRun := rp.Text("▶️ Запустить контроль")
	btnStop := rp.Text("⏹ Остановить")
	btnTZ := rp.Text("🕒 Тайм-зона")
	btnReport := rp.Text("📊 Отчёт")
	rp.Reply(rp.Row(btnAdd, btnList), rp.Row(btnRun, btnStop), rp.Row(btnTZ, btnReport))

	// repeat menu
	a.repMK = &telebot.ReplyMarkup{}
	a.btnRepToday = a.repMK.Data("Сегодня", "rep_today", "today")
	a.btnRepDaily = a.repMK.Data("Ежедневно", "rep_daily", "daily")
	a.btnRepWork = a.repMK.Data("Рабочие дни", "rep_workdays", "workdays")
	a.btnRepCust = a.repMK.Data("Выбрать дни", "rep_custom", "custom")
	a.repMK.Inline(a.repMK.Row(a.btnRepToday, a.btnRepDaily), a.repMK.Row(a.btnRepWork, a.btnRepCust))

	// custom day uniques
	a.btnMon = telebot.Btn{Unique: "day_mon"}
	a.btnTue = telebot.Btn{Unique: "day_tue"}
	a.btnWed = telebot.Btn{Unique: "day_wed"}
	a.btnThu = telebot.Btn{Unique: "day_thu"}
	a.btnFri = telebot.Btn{Unique: "day_fri"}
	a.btnSat = telebot.Btn{Unique: "day_sat"}
	a.btnSun = telebot.Btn{Unique: "day_sun"}
	a.btnDaysDone = telebot.Btn{Unique: "day_done"}

	// report menu
	a.reportMK = &telebot.ReplyMarkup{}
	a.btnRptDay = a.reportMK.Data("Сегодня", "rpt_day", "day")
	a.btnRptWeek = a.reportMK.Data("Неделя", "rpt_week", "week")
	a.btnRptMonth = a.reportMK.Data("Месяц", "rpt_month", "month")
	a.btnRptAll = a.reportMK.Data("Всё время", "rpt_all", "all")
	a.reportMK.Inline(a.reportMK.Row(a.btnRptDay, a.btnRptWeek), a.reportMK.Row(a.btnRptMonth, a.btnRptAll))

	// per-task buttons
	a.btnTaskToggle = telebot.Btn{Unique: "task_toggle"}
	a.btnTaskDelete = telebot.Btn{Unique: "task_delete"}

	// commands
	a.Bot.Handle("/start", func(c telebot.Context) error {
		_, _ = a.St.GetOrCreateUser(c.Sender().ID, defaultTZ)
		return c.Send("Привет! Я помогу контролировать расписание. Используй кнопки ниже или команды /add /list /run /stop /report /help.", rp)
	})
	a.Bot.Handle("/help", func(c telebot.Context) error {
		return c.Send("Команды:\n/add — добавить задачу\n/list — список задач\n/run — запустить контроль\n/stop — остановить контроль\n/tz — сменить тайм-зону\n/report — отчёт по времени")
	})
	a.Bot.Handle(&btnAdd, func(c telebot.Context) error { return a.handleAddStart(c, defaultTZ) })
	a.Bot.Handle("/add", func(c telebot.Context) error { return a.handleAddStart(c, defaultTZ) })
	a.Bot.Handle(&btnList, a.handleList)
	a.Bot.Handle("/list", a.handleList)
	a.Bot.Handle(&btnRun, a.handleRun)
	a.Bot.Handle("/run", a.handleRun)
	a.Bot.Handle(&btnStop, a.handleStop)
	a.Bot.Handle("/stop", a.handleStop)
	a.Bot.Handle(&btnTZ, a.handleTZ)
	a.Bot.Handle("/tz", a.handleTZ)
	a.Bot.Handle(&btnReport, a.handleReportMenu)
	a.Bot.Handle("/report", a.handleReportMenu)

	// inline handlers
	// repeat
	a.Bot.Handle(&a.btnRepToday, func(c telebot.Context) error { return a.cbRepeatChoice(c, "today") })
	a.Bot.Handle(&a.btnRepDaily, func(c telebot.Context) error { return a.cbRepeatChoice(c, "daily") })
	a.Bot.Handle(&a.btnRepWork, func(c telebot.Context) error { return a.cbRepeatChoice(c, "workdays") })
	a.Bot.Handle(&a.btnRepCust, func(c telebot.Context) error { return a.cbRepeatChoice(c, "custom") })
	// custom days
	a.Bot.Handle(&a.btnMon, func(c telebot.Context) error { return a.cbToggleDay(c, time.Monday) })
	a.Bot.Handle(&a.btnTue, func(c telebot.Context) error { return a.cbToggleDay(c, time.Tuesday) })
	a.Bot.Handle(&a.btnWed, func(c telebot.Context) error { return a.cbToggleDay(c, time.Wednesday) })
	a.Bot.Handle(&a.btnThu, func(c telebot.Context) error { return a.cbToggleDay(c, time.Thursday) })
	a.Bot.Handle(&a.btnFri, func(c telebot.Context) error { return a.cbToggleDay(c, time.Friday) })
	a.Bot.Handle(&a.btnSat, func(c telebot.Context) error { return a.cbToggleDay(c, time.Saturday) })
	a.Bot.Handle(&a.btnSun, func(c telebot.Context) error { return a.cbToggleDay(c, time.Sunday) })
	a.Bot.Handle(&a.btnDaysDone, a.cbDaysDone)
	// reports
	a.Bot.Handle(&a.btnRptDay, func(c telebot.Context) error { return a.renderReport(c, "day") })
	a.Bot.Handle(&a.btnRptWeek, func(c telebot.Context) error { return a.renderReport(c, "week") })
	a.Bot.Handle(&a.btnRptMonth, func(c telebot.Context) error { return a.renderReport(c, "month") })
	a.Bot.Handle(&a.btnRptAll, func(c telebot.Context) error { return a.renderReport(c, "all") })
	// per-task list
	a.Bot.Handle(&a.btnTaskToggle, a.cbTaskToggle)
	a.Bot.Handle(&a.btnTaskDelete, a.cbTaskDelete)

	// text for add flow
	a.Bot.Handle(telebot.OnText, a.handleText)
}

func (a *BotApp) handleAddStart(c telebot.Context, defaultTZ string) error {
	_, _ = a.St.GetOrCreateUser(c.Sender().ID, defaultTZ)
	a.addMu.Lock()
	a.addState[c.Sender().ID] = &AddState{Step: 1}
	a.addMu.Unlock()
	return c.Send("Введите название задачи (например: Написать статью)")
}

func (a *BotApp) handleText(c telebot.Context) error {
	text := strings.TrimSpace(c.Text())
	if text == "" {
		return nil
	}
	a.addMu.Lock()
	st, ok := a.addState[c.Sender().ID]
	a.addMu.Unlock()
	if !ok {
		return nil
	}
	switch st.Step {
	case 1:
		st.Title = text
		st.Step = 2
		return c.Send("Время начала (HH:MM), например 09:30")
	case 2:
		h, m, ok := timeutil.ParseHHMM(text)
		if !ok {
			return c.Send("Неверный формат. Введите время как HH:MM (пример: 09:30)")
		}
		st.StartH, st.StartM = h, m
		st.Step = 3
		return c.Send("Время окончания (HH:MM), например 11:00")
	case 3:
		h, m, ok := timeutil.ParseHHMM(text)
		if !ok {
			return c.Send("Неверный формат. Введите время как HH:MM (пример: 11:00)")
		}
		st.EndH, st.EndM = h, m
		if st.EndH < st.StartH || (st.EndH == st.StartH && st.EndM <= st.StartM) {
			return c.Send("Окончание должно быть позже начала. Введите снова время окончания (HH:MM)")
		}
		st.Step = 4
		return c.Send("Выберите повтор:", a.repMK)
	}
	return nil
}

// helpers
func (a *BotApp) formatDays(mask int) string {
	if mask == timeutil.MaskDaily() {
		return "Ежедневно"
	}
	if mask == timeutil.MaskWorkdays() {
		return "Рабочие дни"
	}
	parts := []string{}
	if mask&timeutil.BitMon != 0 {
		parts = append(parts, "Пн")
	}
	if mask&timeutil.BitTue != 0 {
		parts = append(parts, "Вт")
	}
	if mask&timeutil.BitWed != 0 {
		parts = append(parts, "Ср")
	}
	if mask&timeutil.BitThu != 0 {
		parts = append(parts, "Чт")
	}
	if mask&timeutil.BitFri != 0 {
		parts = append(parts, "Пт")
	}
	if mask&timeutil.BitSat != 0 {
		parts = append(parts, "Сб")
	}
	if mask&timeutil.BitSun != 0 {
		parts = append(parts, "Вс")
	}
	if len(parts) == 0 {
		return "-"
	}
	if len(parts) == 1 {
		return "Сегодня (" + parts[0] + ")"
	}
	return strings.Join(parts, ", ")
}

func (a *BotApp) renderCustomDaysKeyboard(st *AddState) *telebot.ReplyMarkup {
	isOn := func(bit int) bool { return (st.DaysMask & bit) != 0 }
	label := func(name string, on bool) string {
		if on {
			return "✅ " + name
		}
		return name
	}
	mk := &telebot.ReplyMarkup{}
	bMon := mk.Data(label("Пн", isOn(timeutil.BitMon)), a.btnMon.Unique, "mon")
	bTue := mk.Data(label("Вт", isOn(timeutil.BitTue)), a.btnTue.Unique, "tue")
	bWed := mk.Data(label("Ср", isOn(timeutil.BitWed)), a.btnWed.Unique, "wed")
	bThu := mk.Data(label("Чт", isOn(timeutil.BitThu)), a.btnThu.Unique, "thu")
	bFri := mk.Data(label("Пт", isOn(timeutil.BitFri)), a.btnFri.Unique, "fri")
	bSat := mk.Data(label("Сб", isOn(timeutil.BitSat)), a.btnSat.Unique, "sat")
	bSun := mk.Data(label("Вс", isOn(timeutil.BitSun)), a.btnSun.Unique, "sun")
	bDone := mk.Data("Готово", a.btnDaysDone.Unique, "done")
	mk.Inline(mk.Row(bMon, bTue, bWed, bThu), mk.Row(bFri, bSat, bSun), mk.Row(bDone))
	return mk
}

func (a *BotApp) buildTaskText(t store.Task) string {
	state := "ВЫКЛ"
	if t.Enabled {
		state = "ВКЛ"
	}
	return fmt.Sprintf("• %02d:%02d–%02d:%02d %s [%s]\nДни: %s", t.StartH, t.StartM, t.EndH, t.EndM, t.Title, state, a.formatDays(t.DaysMask))
}

func (a *BotApp) buildTaskMarkup(t store.Task) *telebot.ReplyMarkup {
	mk := &telebot.ReplyMarkup{}
	bT := mk.Data("Вкл/Выкл", a.btnTaskToggle.Unique, fmt.Sprintf("%d", t.ID))
	bD := mk.Data("Удалить", a.btnTaskDelete.Unique, fmt.Sprintf("%d", t.ID))
	mk.Inline(mk.Row(bT, bD))
	return mk
}

// callbacks

func (a *BotApp) cbRepeatChoice(c telebot.Context, kind string) error {
	id := c.Sender().ID
	a.addMu.Lock()
	st, ok := a.addState[id]
	a.addMu.Unlock()
	if !ok {
		return c.Respond()
	}

	switch kind {
	case "today":
		u, err := a.St.GetUserByTGID(id)
		if err != nil {
			return c.Respond()
		}
		loc, err := time.LoadLocation(u.TZ)
		if err != nil {
			return c.Respond()
		}
		now := time.Now().In(loc)
		st.DaysMask = timeutil.WeekdayBit(now.Weekday())
		return a.finishAdd(c)
	case "daily":
		st.DaysMask = timeutil.MaskDaily()
		return a.finishAdd(c)
	case "workdays":
		st.DaysMask = timeutil.MaskWorkdays()
		return a.finishAdd(c)
	case "custom":
		return c.Edit(chooseDaysTitle, a.renderCustomDaysKeyboard(st))
	}
	return c.Respond()
}

func (a *BotApp) cbToggleDay(c telebot.Context, wd time.Weekday) error {
	id := c.Sender().ID
	a.addMu.Lock()
	st, ok := a.addState[id]
	if ok {
		st.DaysMask ^= timeutil.WeekdayBit(wd)
	}
	a.addMu.Unlock()
	if ok {
		_ = c.Edit(chooseDaysTitle, a.renderCustomDaysKeyboard(st))
		return c.Respond()
	}
	return c.Respond(&telebot.CallbackResponse{Text: "Нет активного добавления"})
}

func (a *BotApp) cbDaysDone(c telebot.Context) error {
	id := c.Sender().ID
	a.addMu.Lock()
	st, ok := a.addState[id]
	a.addMu.Unlock()
	if !ok || st.DaysMask == 0 {
		return c.Respond(&telebot.CallbackResponse{Text: "Выберите хотя бы один день"})
	}
	return a.finishAdd(c)
}

func (a *BotApp) cbTaskToggle(c telebot.Context) error {
	id := c.Sender().ID
	u, err := a.St.GetUserByTGID(id)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Ошибка пользователя"})
	}
	var taskID int64
	fmt.Sscanf(c.Callback().Data, "%d", &taskID)
	enabled, err := a.St.ToggleTask(u.ID, taskID)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Ошибка"})
	}
	if u.ControlEnabled {
		_ = a.Sch.ScheduleAllForUser(u)
	}
	t, err := a.St.GetTask(u.ID, taskID)
	if err == nil {
		_ = c.Edit(a.buildTaskText(t), a.buildTaskMarkup(t))
	}
	msg := "Выключено"
	if enabled {
		msg = "Включено"
	}
	return c.Respond(&telebot.CallbackResponse{Text: msg})
}

func (a *BotApp) cbTaskDelete(c telebot.Context) error {
	id := c.Sender().ID
	u, err := a.St.GetUserByTGID(id)
	if err != nil {
		return c.Respond(&telebot.CallbackResponse{Text: "Ошибка пользователя"})
	}
	var taskID int64
	fmt.Sscanf(c.Callback().Data, "%d", &taskID)
	_ = a.St.DeleteTask(u.ID, taskID)
	if u.ControlEnabled {
		_ = a.Sch.ScheduleAllForUser(u)
	}
	_ = c.Delete()
	return nil
}

// command handlers

func (a *BotApp) finishAdd(c telebot.Context) error {
	id := c.Sender().ID
	u, err := a.St.GetUserByTGID(id)
	if err != nil {
		return c.Send("Ошибка пользователя")
	}
	a.addMu.Lock()
	st := a.addState[id]
	delete(a.addState, id)
	a.addMu.Unlock()
	if st == nil {
		return c.Send("Отменено")
	}

	taskID, err := a.St.CreateTask(store.Task{
		UserID: u.ID, Title: st.Title,
		StartH: st.StartH, StartM: st.StartM,
		EndH: st.EndH, EndM: st.EndM,
		DaysMask: st.DaysMask,
	})
	if err != nil {
		return c.Send("Не удалось сохранить задачу")
	}
	if u.ControlEnabled {
		_ = a.Sch.ScheduleAllForUser(u)
	}

	t, _ := a.St.GetTask(u.ID, taskID)
	return c.Send("✅Задача добавлена.\n ⚠️Не забудь запустить контроль!\n" + a.buildTaskText(t))
}

func (a *BotApp) handleList(c telebot.Context) error {
	u, err := a.St.GetUserByTGID(c.Sender().ID)
	if err != nil {
		return c.Send("Сначала /start")
	}
	tasks, err := a.St.ListTasks(u.ID)
	if err != nil {
		return c.Send("Ошибка чтения задач")
	}
	if len(tasks) == 0 {
		return c.Send("Задач пока нет. Нажми ➕ Добавить задачу")
	}
	for _, t := range tasks {
		_ = c.Send(a.buildTaskText(t), a.buildTaskMarkup(t))
	}
	return nil
}

func (a *BotApp) handleRun(c telebot.Context) error {
	u, err := a.St.GetUserByTGID(c.Sender().ID)
	if err != nil {
		return c.Send("Сначала /start")
	}
	if err := a.St.SetControl(u.ID, true); err != nil {
		return c.Send("Ошибка")
	}
	u.ControlEnabled = true
	if err := a.Sch.ScheduleAllForUser(u); err != nil {
		log.Println("schedule error:", err)
	}
	return c.Send("Контроль запущен. Буду присылать уведомления по задачам на сегодня. Перепланирование выполняется каждый день в 00:05 вашей тайм-зоны.")
}

func (a *BotApp) handleStop(c telebot.Context) error {
	u, err := a.St.GetUserByTGID(c.Sender().ID)
	if err != nil {
		return c.Send("Сначала /start")
	}
	if err := a.St.SetControl(u.ID, false); err != nil {
		return c.Send("Ошибка")
	}
	a.Sch.ClearUser(u.ID)
	return c.Send("Контроль остановлен. Рассылка уведомлений отключена.")
}

func (a *BotApp) handleTZ(c telebot.Context) error {
	u, err := a.St.GetUserByTGID(c.Sender().ID)
	if err != nil {
		return c.Send("Сначала /start")
	}
	args := strings.Fields(c.Message().Payload)
	if len(args) == 0 {
		return c.Send("Отправьте команду так: /tz Europe/Kyiv (или отправьте свою IANA тайм-зону)")
	}
	tz := args[0]
	if _, err := time.LoadLocation(tz); err != nil {
		return c.Send("Неверная тайм-зона. Пример: Europe/Kyiv")
	}
	if err := a.St.UpdateUserTZ(u.ID, tz); err != nil {
		return c.Send("Ошибка сохранения TZ")
	}
	if u.ControlEnabled {
		_ = a.Sch.ScheduleAllForUser(store.User{ID: u.ID, TGID: u.TGID, TZ: tz, ControlEnabled: true})
	}
	return c.Send("Тайм-зона обновлена: " + tz)
}

func (a *BotApp) handleReportMenu(c telebot.Context) error {
	return c.Send("Выберите период отчёта:", a.reportMK)
}

func (a *BotApp) renderReport(c telebot.Context, period string) error {
	u, err := a.St.GetUserByTGID(c.Sender().ID)
	if err != nil {
		return c.Send("Сначала /start")
	}
	fromUTC, toUTC, err := timeutil.RangeUTC(period, u.TZ)
	if err != nil {
		return c.Send("Ошибка тайм-зоны")
	}
	stats, err := a.St.GetStats(u.ID, fromUTC, toUTC)
	if err != nil {
		return c.Send("Ошибка отчёта")
	}
	if len(stats) == 0 {
		switch period {
		case "day":
			return c.Edit("За сегодня пока нет данных.")
		case "week":
			return c.Edit("За эту неделю пока нет данных.")
		case "month":
			return c.Edit("За этот месяц пока нет данных.")
		default:
			return c.Edit("Данных пока нет.")
		}
	}
	sort.Slice(stats, func(i, j int) bool { return stats[i].Seconds > stats[j].Seconds })
	total := int64(0)
	for _, s := range stats {
		total += s.Seconds
	}
	var b strings.Builder
	title := map[string]string{"day": "сегодня", "week": "неделю", "month": "месяц", "all": "всё время"}[period]
	if title == "" {
		title = "период"
	}
	fmt.Fprintf(&b, "Отчёт за %s:\n", title)
	for _, s := range stats {
		h := s.Seconds / 3600
		m := (s.Seconds % 3600) / 60
		fmt.Fprintf(&b, "• %s — %02dч %02dм\n", s.Title, h, m)
	}
	h := total / 3600
	m := (total % 3600) / 60
	fmt.Fprintf(&b, "\nИтого: %02dч %02dм", h, m)
	return c.Edit(b.String())
}
