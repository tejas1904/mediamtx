package record

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/stream"
)

// Agent writes recordings to disk.
type Agent struct {
	WriteQueueSize    int
	PathFormat        string
	Format            conf.RecordFormat
	PartDuration      time.Duration
	SegmentDuration   time.Duration
	PathName          string
	Stream            *stream.Stream
	OnSegmentCreate   OnSegmentFunc
	OnSegmentComplete OnSegmentFunc
	Parent            logger.Writer

	restartPause time.Duration

	currentInstance *agentInstance

	terminate chan struct{}
	done      chan struct{}
}

// Initialize initializes Agent.
func (w *Agent) Initialize() {
	if w.OnSegmentCreate == nil {
		w.OnSegmentCreate = func(string) {
		}
	}
	if w.OnSegmentComplete == nil {
		w.OnSegmentComplete = func(string) {
		}
	}
	if w.restartPause == 0 {
		w.restartPause = 2 * time.Second
	}

	w.terminate = make(chan struct{})
	w.done = make(chan struct{})

	w.currentInstance = &agentInstance{
		agent: w,
	}
	w.currentInstance.initialize()

	go w.run()
}

// Log implements logger.Writer.
func (w *Agent) Log(level logger.Level, format string, args ...interface{}) {
	w.Parent.Log(level, "[record] "+format, args...)
}

// Close closes the agent.
func (w *Agent) Close() {
	w.Log(logger.Info, "recording stopped")
	close(w.terminate)
	<-w.done
}

func (w *Agent) run() {
	defer close(w.done)

	for {
		select {
		case <-w.currentInstance.done:
			w.currentInstance.close()
		case <-w.terminate:
			w.currentInstance.close()
			return
		}

		select {
		case <-time.After(w.restartPause):
		case <-w.terminate:
			return
		}

		w.currentInstance = &agentInstance{
			agent: w,
		}
		w.currentInstance.initialize()
	}
}

//custom onSegmentComplete

func (w *Agent) CustomOnSegmentComplete(path string) {
	dir := filepath.Dir(path)
	subpath := filepath.Base(filepath.Dir(path)) // Retrieve the second-to-last element
	filename := filepath.Base(path)

	filePath := dir + "/" + subpath + ".m3u8"

	var file *os.File
	var err error

	// Check if the file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// If the file does not exist, create it
		file, err = os.Create(filePath)
		if err != nil {
			w.Log(logger.Error, "Error creating .m3u8 file: %s", filePath)
			return
		}
		defer file.Close()

		// Write initial content to the file
		initialContent := "#EXTM3U" + "\n" + "#EXT-X-VERSION:3" + "\n"
		_, err = file.WriteString(initialContent)
		if err != nil {
			w.Log(logger.Error, "Error writing initial content to file: %s", filePath)
			return
		}
	} else {
		// If the file exists, open it in append mode
		file, err = os.OpenFile(filePath, os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			w.Log(logger.Error, "Error opening file: %s", filePath)
			return
		}
		defer file.Close()
	}

	// Write content to the file
	content := "#EXTINF:" + w.SegmentDuration.String() + "," + "\n" + filename + "\n"
	_, err = file.WriteString(content)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}
