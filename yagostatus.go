package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/burik666/yagostatus/ygs"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
)

type YaGoStatus struct {
	widgets       []ygs.Widget
	widgetsOutput [][]ygs.I3BarBlock
	widgetsConfig []ConfigWidget
	upd           chan int
}

func (status *YaGoStatus) AddWidget(widget ygs.Widget, config ConfigWidget) error {
	if err := widget.Configure(config.Params); err != nil {
		return err
	}
	status.widgets = append(status.widgets, widget)
	status.widgetsOutput = append(status.widgetsOutput, nil)
	status.widgetsConfig = append(status.widgetsConfig, config)

	return nil
}

func (status *YaGoStatus) processWidgetEvents(i int, j int, event ygs.I3BarClickEvent) error {
	defer status.widgets[i].Event(event)
	for _, e := range status.widgetsConfig[i].Events {
		if (e.Button == 0 || event.Button == e.Button) &&
			(e.Name == "" || e.Name == event.Name) &&
			(e.Instance == "" || e.Instance == event.Instance) {
			cmd := exec.Command("sh", "-c", e.Command)
			cmd.Stderr = os.Stderr
			cmd.Env = append(os.Environ(),
				fmt.Sprintf("I3_%s=%s", "NAME", event.Name),
				fmt.Sprintf("I3_%s=%s", "INSTANCE", event.Instance),
				fmt.Sprintf("I3_%s=%d", "BUTTON", event.Button),
				fmt.Sprintf("I3_%s=%d", "X", event.X),
				fmt.Sprintf("I3_%s=%d", "Y", event.Y),
			)
			cmdStdin, err := cmd.StdinPipe()
			if err != nil {
				return err
			}
			eventJson, _ := json.Marshal(event)
			cmdStdin.Write(eventJson)
			cmdStdin.Write([]byte("\n"))
			cmdStdin.Close()

			cmdOutput, err := cmd.Output()
			if err != nil {
				return err
			}
			if e.Output {
				var blocks []ygs.I3BarBlock
				if err := json.Unmarshal(cmdOutput, &blocks); err == nil {
					for bi, _ := range blocks {
						block := &blocks[bi]
						MergeBlocks(block, status.widgetsConfig[i].Template)
						block.Name = fmt.Sprintf("ygs-%d-%s", i, block.Name)
						block.Instance = fmt.Sprintf("ygs-%d-%d-%s", i, j, block.Instance)
					}
					status.widgetsOutput[i] = blocks
				} else {
					status.widgetsOutput[i][j].FullText = strings.Trim(string(cmdOutput), "\n\r")
				}
				status.upd <- i
			}
		}
	}
	return nil
}

func (status *YaGoStatus) eventReader() {
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err != io.EOF {
				log.Fatal(err)
			}
			break
		}
		line = strings.Trim(line, "[], \n")
		if line != "" {
			var event ygs.I3BarClickEvent
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				log.Printf("%s (%s)", err, line)
			} else {
				for i, widgetOutput := range status.widgetsOutput {
					for j, output := range widgetOutput {
						if (event.Name == output.Name) && (event.Instance == output.Instance) {
							e := event
							e.Name = strings.Join(strings.Split(e.Name, "-")[2:], "-")
							e.Instance = strings.Join(strings.Split(e.Instance, "-")[3:], "-")
							if err := status.processWidgetEvents(i, j, e); err != nil {
								log.Printf("%s", err)
							}
						}
					}
				}
			}
		}
	}
}

func (status *YaGoStatus) Run() {
	status.upd = make(chan int)
	for i, widget := range status.widgets {
		c := make(chan []ygs.I3BarBlock)
		go func(i int, c chan []ygs.I3BarBlock) {
			for {
				select {
				case out := <-c:
					output := make([]ygs.I3BarBlock, len(out))
					copy(output, out)
					for j, _ := range output {
						MergeBlocks(&output[j], status.widgetsConfig[i].Template)
						output[j].Name = fmt.Sprintf("ygs-%d-%s", i, output[j].Name)
						output[j].Instance = fmt.Sprintf("ygs-%d-%d-%s", i, j, output[j].Instance)
					}
					status.widgetsOutput[i] = output
					status.upd <- i
				}
			}
		}(i, c)

		go func(widget ygs.Widget, c chan []ygs.I3BarBlock) {
			if err := widget.Run(c); err != nil {
				log.Print(err)
				c <- []ygs.I3BarBlock{ygs.I3BarBlock{
					FullText: err.Error(),
					Urgent:   true,
				}}
			}
		}(widget, c)
	}

	fmt.Print("{\"version\":1, \"click_events\": true}\n[\n[]")
	go func() {
		buf := &bytes.Buffer{}
		encoder := json.NewEncoder(buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		for {
			select {
			case <-status.upd:
				var result []ygs.I3BarBlock
				for _, widgetOutput := range status.widgetsOutput {
					result = append(result, widgetOutput...)
				}
				buf.Reset()
				encoder.Encode(result)
				fmt.Print(",")
				fmt.Print(string(buf.Bytes()))
			}
		}
	}()
	status.eventReader()
}

func (status *YaGoStatus) Stop() {
	var wg sync.WaitGroup
	for _, widget := range status.widgets {
		wg.Add(1)
		go func(widget ygs.Widget) {
			widget.Stop()
			wg.Done()
		}(widget)
	}
	wg.Wait()
}

func MergeBlocks(b *ygs.I3BarBlock, tpl ygs.I3BarBlock) {
	var resmap map[string]interface{}

	jb, _ := json.Marshal(*b)
	jtpl, _ := json.Marshal(tpl)
	json.Unmarshal(jtpl, &resmap)
	json.Unmarshal(jb, &resmap)

	jb, _ = json.Marshal(resmap)
	json.Unmarshal(jb, b)
}
