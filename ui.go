package main

import (
	"fmt"
	ui "github.com/gizak/termui"
	"golang.org/x/net/context"
	"log"
	"time"
	"unicode"
)

func runUI(ctx context.Context, name string, memberc <-chan []string, msgc <-chan []message) (<-chan message, error) {

	if err := ui.Init(); err != nil {
		return nil, fmt.Errorf("can't initialize UI: %v", err)
	}

	chatc := make(chan message, 1)

	go func() {

		defer ui.Close()
		defer close(chatc)
		mlistBox := ui.NewList()
		mlistBox.Height = 11
		mlistBox.Border.Label = "members"
		mlistBox.ItemFgColor = ui.ColorYellow

		msgBox := ui.NewList()
		msgBox.Height = 11
		msgBox.Border.Label = "chat"
		msgBox.ItemFgColor = ui.ColorWhite

		inputBox := ui.NewPar("")
		inputBox.Border.Label = name
		inputBox.Height = 3

		var msgHistory []message
		updateHeights := func(screenHeight int) {
			mlistBox.Height = screenHeight - 3
			msgBox.Height = screenHeight - 3
			inputBox.Height = 3
		}

		updateHeights(ui.TermHeight())

		ui.Body.AddRows(
			ui.NewRow(
				ui.NewCol(3, 0, mlistBox),
				ui.NewCol(9, 0, msgBox),
			),
			ui.NewCol(12, 0, inputBox),
		)

		ui.Body.Align()

		log.Printf("first draw")
		ui.Render(ui.Body)

		redraw := make(chan struct{})

		setMsgItems := func(msgs []message) {
			msgHistory = msgs
			msgBox.Items = make([]string, 0, len(msgs))
			for _, msg := range msgs {
				msgBox.Items = append(msgBox.Items, fmt.Sprintf("%s %s: %s",
					msg.Time.Format(time.Kitchen), msg.Author, msg.Msg,
				))
			}
			if len(msgBox.Items) > msgBox.Height {
				maxFit := msgBox.Height - 2
				msgBox.Items = msgBox.Items[len(msgBox.Items)-maxFit:]
			}
		}

		evc := ui.EventCh()
		for {
			select {

			case member := <-memberc:
				mlistBox.Items = member
				go func() { redraw <- struct{}{} }()

			case msgs := <-msgc:
				setMsgItems(msgs)
				go func() { redraw <- struct{}{} }()

			case e := <-evc:
				switch e.Type {
				case ui.EventKey:

					switch e.Key {
					case ui.KeyCtrlC:
						return
					case ui.KeyEnter:
						author := inputBox.Border.Label
						body := inputBox.Text

						msg := newMessage(author, body)
						inputBox.Text = ""
						go func() { chatc <- msg }()
						go func() { redraw <- struct{}{} }()
					case ui.KeySpace:
						inputBox.Text += " "
						go func() { redraw <- struct{}{} }()
					case ui.KeyBackspace, ui.KeyBackspace2:
						if len(inputBox.Text) != 0 {
							inputBox.Text = inputBox.Text[:len(inputBox.Text)-1]
							go func() { redraw <- struct{}{} }()
						}

					case 0:
						switch {
						case unicode.IsPrint(e.Ch):
							inputBox.Text += string(e.Ch)
							go func() { redraw <- struct{}{} }()

						default:
							log.Printf("unknown key=%q", e.Ch)
						}

					default:
						log.Printf("e.Key=%#v", e.Key)

					}

				case ui.EventResize:
					ui.Body.Width = ui.TermWidth()
					updateHeights(ui.TermHeight())
					setMsgItems(msgHistory)
					ui.Body.Align()
					go func() { redraw <- struct{}{} }()

				default:
					log.Printf("event=%#v", e)
				}

			case <-ctx.Done():
				return
			case <-redraw:
				ui.Render(ui.Body)
			}
		}
	}()
	return chatc, nil
}
