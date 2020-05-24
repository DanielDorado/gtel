package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne"

	"fyne.io/fyne/app"
	"fyne.io/fyne/widget"
	"gopkg.in/yaml.v2"
)

const (
	BEGIN              = "BEGIN"
	END                = "END"
	TITLE              = "* GoL Time Emit Log *"
	CONFIGURATION_FILE = ".goltime.yml"
)

type Configuration struct {
	MusicCommand      string   `yaml:"musicCommand"`
	CountdownOptions  []int    `yaml:"countdownOptions"`
	CountdownDefault  int      `yaml:"countdownDefault"`
	Options           []string `yaml:"options"`
	TrackingDirectory string   `yaml:"trackingDirectory"`
	Tasks             []Task   `tasks`
}

type Context struct {
	MusicCommand      string
	CurrentTaskButton *TaskButton
	StopButton        *widget.Button
	CountdownMinutes  int
	CountdownOptions  []int
	CountdownDefault  int
	TrackingDirectory string
	StopTimerChannel  chan bool
	BeginEndMutex     sync.Mutex
	Options           []string
}

type Task struct {
	JiraCode      string `yaml:"jiraCode"`
	DefaultOption string `yaml:"defaultOption"`
	Option        string `yaml:"option"`
	Summary       string `yaml:"summary"`
}

type TaskButton struct {
	Task
	Button *widget.Button
	Select *widget.Select
}

func (tb TaskButton) SetButtonText() {
	tb.Button.Text = fmt.Sprintf("%s %s > %s", tb.JiraCode, tb.Summary, tb.Option)
	fmt.Println("Button text: " + tb.Button.Text)
	tb.Button.Refresh()
}

func beginTask(ctx *Context, tb *TaskButton) {
	actionTask(ctx, BEGIN, tb.JiraCode, tb.Option, tb.Summary, now())
}

func endTask(ctx *Context, tb *TaskButton) {
	actionTask(ctx, END, tb.JiraCode, tb.Option, tb.Summary, now())
}

func now() string {
	return time.Now().Format("2006-01-02 15:04:05")
}

func actionTask(ctx *Context, action, jiraCode, option, summary, now string) {
	logLine := fmt.Sprintf("%s,  %d,   %s   %s.  %s %s\n", action, ctx.CountdownDefault, now, jiraCode, option, summary)
	fmt.Println(logLine)
	file := path.Join(ctx.TrackingDirectory, "tracking_"+time.Now().Format("2006_01_02")+".csv")
	f, _ := os.OpenFile(file, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0660)
	f.WriteString(logLine)
}

func beginTimer(ctx *Context) {
	var timerChannel = make(<-chan time.Time)
	if ctx.CountdownMinutes != 0 {
		fmt.Printf("Beginning countdown: %d minutes\n", ctx.CountdownMinutes)
		timerChannel = time.After(time.Duration(ctx.CountdownMinutes) * time.Minute)
	}
	current := ctx.CurrentTaskButton
	select {
	case <-ctx.StopTimerChannel:
		fmt.Println("INTERRUPT TIMER ---------------------------------------------------------------")
		endTask(ctx, current)
		current.Button.Enable()
		ctx.BeginEndMutex.Unlock()
	case <-timerChannel:
		fmt.Println("TIMER ---------------------------------------------------------------")
		endTask(ctx, current)
		current.Button.Enable()
		ctx.BeginEndMutex.Unlock()
		command := strings.Split(ctx.MusicCommand, " ")
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Start()
		select {
		case <-ctx.StopTimerChannel:
			fmt.Println("PLAYER STOPPED ---------------------------------------------------------------")
			cmd.Process.Kill()
		case <-time.After(3 * time.Minute):
			fmt.Println("PLAYER TIMEOUT ---------------------------------------------------------------")
			cmd.Process.Kill()
			ctx.StopButton.Disable()
		}
	}
}

func GetStartFunc(btn *TaskButton, ctx *Context) func() {
	return func() {
		// stop timer or player
		if ctx.StopTimerChannel != nil {
			close(ctx.StopTimerChannel)
			ctx.StopTimerChannel = nil
		}
		ctx.BeginEndMutex.Lock()
		ctx.CurrentTaskButton = btn
		beginTask(ctx, btn)
		ctx.StopTimerChannel = make(chan bool)
		go beginTimer(ctx)
		// }
		ctx.StopButton.Enable()
		btn.Button.Disable()
	}
}

func GetSelectFunc(btn *TaskButton, ctx *Context) func(string) {
	return func(selected string) {
		fmt.Println("Option: " + selected)
		btn.Option = selected
		btn.SetButtonText()
		// btn.Button.Refresh()
	}
}

func GetStopFunc(ctx *Context) func() {
	return func() {
		// stop timer or player
		if ctx.StopTimerChannel != nil {
			close(ctx.StopTimerChannel)
			ctx.StopTimerChannel = nil
		}
		ctx.StopButton.Disable()
	}
}

func GetQuitFunc(ctx *Context, app fyne.App) func() {
	return func() {
		// stop timer or player
		if ctx.StopTimerChannel != nil {
			close(ctx.StopTimerChannel)
			ctx.StopTimerChannel = nil
		}
		fmt.Printf("Quit app!")
		app.Quit()
	}
}

func main() {
	config := Configuration{}
	usr, _ := user.Current()
	configurationFile := filepath.Join(usr.HomeDir, CONFIGURATION_FILE)
	fmt.Println("Configuration file: " + configurationFile)
	content, _ := ioutil.ReadFile(configurationFile)
	err := yaml.Unmarshal(content, &config)
	if err != nil {
		panic("Error unmarshalling configuration:" + err.Error())
	}

	app := app.New()

	w := app.NewWindow("Hello")

	ctx := Context{}
	ctx.Options = config.Options
	ctx.MusicCommand = config.MusicCommand
	ctx.CountdownDefault = config.CountdownDefault
	ctx.CountdownMinutes = config.CountdownDefault
	ctx.CountdownOptions = config.CountdownOptions
	ctx.TrackingDirectory = config.TrackingDirectory

	os.MkdirAll(ctx.TrackingDirectory, 0770)

	var taskButtons []*TaskButton
	for _, t := range config.Tasks {
		button := widget.NewButton("BUTTON WITHOUT TEXT", nil)
		selector := widget.NewSelect(ctx.Options, nil)
		selector.Selected = t.DefaultOption
		tb := &TaskButton{t, button, selector}
		button.OnTapped = GetStartFunc(tb, &ctx)
		selector.OnChanged = GetSelectFunc(tb, &ctx)
		tb.Option = tb.DefaultOption
		tb.SetButtonText()
		taskButtons = append(taskButtons, tb)
	}

	verticalBox := widget.NewVBox()
	verticalBox.Append(widget.NewLabel(TITLE))
	timeBox := widget.NewHBox()
	var options []string
	for _, i := range ctx.CountdownOptions {
		options = append(options, strconv.Itoa(i))
		fmt.Printf("Countdown set to: %d\n", ctx.CountdownMinutes)
	}
	countdownMinutes := widget.NewSelect(options, func(value string) {
		minutes, _ := strconv.Atoi(value)
		ctx.CountdownMinutes = minutes
		fmt.Printf("Countdown set to: %d\n", ctx.CountdownMinutes)
	})
	countdownMinutes.Selected = strconv.Itoa(ctx.CountdownDefault)
	timeBox.Append(countdownMinutes)
	timeBox.Append(widget.NewLabel("<-- Countdown timer configuration (0 => no countdown)"))

	verticalBox.Append(timeBox)
	for _, tb := range taskButtons {
		hb := widget.NewHBox()
		hb.Append(tb.Select)
		hb.Append(tb.Button)
		verticalBox.Append(hb)
	}

	ctx.StopButton = widget.NewButton("Stop", GetStopFunc(&ctx))
	verticalBox.Append(ctx.StopButton)
	ctx.StopButton.Disable()
	verticalBox.Append(widget.NewButton("Quit", GetQuitFunc(&ctx, app)))
	w.SetContent(verticalBox)
	height := w.Canvas().Size().Height
	width := w.Canvas().Size().Width
	fmt.Println(w.Canvas().Size())
	if height < 500 {
		// w.Resize(fyne.Size{Height: 240})
		w.Resize(fyne.Size{Height: 520, Width: width})
	}
	w.SetOnClosed(func() {
		GetStopFunc(&ctx)()
		time.Sleep(2 * time.Second)
	})
	w.ShowAndRun()
}
