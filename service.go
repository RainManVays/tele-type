package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Status represents the current typing state.
type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusPaused  Status = "paused"
	StatusDone    Status = "done"
)

// FileInfo holds metadata about the loaded file.
type FileInfo struct {
	Path         string  `json:"path"`
	MD5          string  `json:"md5"`
	SizeKB       float64 `json:"sizeKB"`
	Base64Path   string  `json:"base64Path"`
	Base64SizeKB float64 `json:"base64SizeKB"`
	OffsetPath   string  `json:"offsetPath"`
	ResumedFrom  int     `json:"resumedFrom"`
}

// Progress carries the current typing progress snapshot.
type Progress struct {
	Current          int     `json:"current"`
	Total            int     `json:"total"`
	Percent          float64 `json:"percent"`
	SpeedCharsPerMin int     `json:"speedCharsPerMin"`
	SpeedBytesPerSec int     `json:"speedBytesPerSec"`
	RemainingSeconds int     `json:"remainingSeconds"`
	Status           Status  `json:"status"`
}

// TeleTypeService is the Wails service that exposes typing operations to the frontend.
type TeleTypeService struct {
	mu           sync.Mutex
	fileInfo     *FileInfo
	encodedText  string
	currentIndex int
	status       Status
	cancelChan   chan struct{}
	startTime    time.Time
}

func NewTeleTypeService() *TeleTypeService {
	return &TeleTypeService{status: StatusIdle}
}

// OpenFileDialog opens a native file-picker and returns the selected path.
func (s *TeleTypeService) OpenFileDialog() (string, error) {
	return application.Get().Dialog.OpenFile().
		CanChooseFiles(true).
		PromptForSingleSelection()
}

// LoadFile encodes the given file to Base64, reads any saved offset, and
// returns metadata. Must be called before Start.
func (s *TeleTypeService) LoadFile(path string) (*FileInfo, error) {
	if path == "" {
		return nil, fmt.Errorf("no file path provided")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.status == StatusRunning || s.status == StatusPaused {
		return nil, fmt.Errorf("cannot load a file while typing is active")
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("error getting absolute path: %w", err)
	}

	md5Sum, err := calculateMD5(absPath)
	if err != nil {
		return nil, fmt.Errorf("error calculating MD5: %w", err)
	}

	stat, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	base64Path := absPath + ".txt"
	offsetPath := absPath + ".offset"

	if err := createBase64File(absPath, base64Path); err != nil {
		return nil, fmt.Errorf("error creating base64 file: %w", err)
	}

	b64Stat, err := os.Stat(base64Path)
	if err != nil {
		return nil, fmt.Errorf("error reading base64 file info: %w", err)
	}

	data, err := os.ReadFile(base64Path)
	if err != nil {
		return nil, fmt.Errorf("error reading base64 content: %w", err)
	}

	offset := loadOffset(offsetPath)
	s.encodedText = string(data)
	s.currentIndex = offset
	s.status = StatusIdle
	s.fileInfo = &FileInfo{
		Path:         absPath,
		MD5:          md5Sum,
		SizeKB:       float64(stat.Size()) / 1024,
		Base64Path:   base64Path,
		Base64SizeKB: float64(b64Stat.Size()) / 1024,
		OffsetPath:   offsetPath,
		ResumedFrom:  offset,
	}

	return s.fileInfo, nil
}

// Start begins the typing simulation. Starts a countdown of delaySeconds before typing.
func (s *TeleTypeService) Start(delaySeconds int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fileInfo == nil {
		return fmt.Errorf("no file loaded")
	}
	if s.status == StatusRunning {
		return fmt.Errorf("already running")
	}
	if s.status == StatusDone {
		return fmt.Errorf("typing complete — load a new file to start again")
	}

	s.status = StatusRunning
	s.cancelChan = make(chan struct{})
	s.startTime = time.Now()

	go s.runTyping(s.encodedText, s.fileInfo.OffsetPath, s.cancelChan, delaySeconds)
	return nil
}

// Pause suspends the typing loop without losing progress.
func (s *TeleTypeService) Pause() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusRunning {
		s.status = StatusPaused
	}
}

// Resume continues a paused typing session.
func (s *TeleTypeService) Resume() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status == StatusPaused {
		s.status = StatusRunning
	}
}

// Cancel stops typing and deletes the generated .txt and .offset files.
func (s *TeleTypeService) Cancel() {
	s.mu.Lock()
	ch := s.cancelChan
	fi := s.fileInfo
	s.cancelChan = nil
	s.fileInfo = nil
	s.encodedText = ""
	s.currentIndex = 0
	s.status = StatusIdle
	s.mu.Unlock()

	if ch != nil {
		close(ch)
	}
	if fi != nil {
		os.Remove(fi.Base64Path)
		os.Remove(fi.OffsetPath)
	}
}

// GetProgress returns the current progress snapshot.
func (s *TeleTypeService) GetProgress() Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snapshot()
}

// snapshot builds a Progress value; must be called with s.mu held.
func (s *TeleTypeService) snapshot() Progress {
	total := len(s.encodedText)
	p := Progress{
		Current: s.currentIndex,
		Total:   total,
		Status:  s.status,
	}
	if total > 0 {
		p.Percent = float64(s.currentIndex) / float64(total) * 100
	}
	if !s.startTime.IsZero() && s.currentIndex > 0 {
		elapsed := time.Since(s.startTime).Seconds()
		if elapsed > 0 {
			p.SpeedCharsPerMin = int(float64(s.currentIndex) / elapsed * 60)
			p.SpeedBytesPerSec = int(float64(s.currentIndex) / elapsed)
			remaining := total - s.currentIndex
			if p.SpeedCharsPerMin > 0 {
				p.RemainingSeconds = int(float64(remaining) / (float64(p.SpeedCharsPerMin) / 60))
			}
		}
	}
	return p
}

func (s *TeleTypeService) runTyping(text string, offsetPath string, cancelChan chan struct{}, delaySeconds int) {
	app := application.Get()
	total := len(text)

	// Countdown before typing begins.
	for i := delaySeconds; i > 0; i-- {
		select {
		case <-cancelChan:
			return
		default:
		}
		app.Event.Emit("teletype:countdown", i)
		time.Sleep(time.Second)
	}

	s.mu.Lock()
	s.startTime = time.Now() // reset timer after countdown
	s.mu.Unlock()

	for {
		select {
		case <-cancelChan:
			return
		default:
		}

		s.mu.Lock()
		status := s.status
		idx := s.currentIndex
		s.mu.Unlock()

		if idx >= total {
			s.mu.Lock()
			s.status = StatusDone
			prog := s.snapshot()
			s.mu.Unlock()
			app.Event.Emit("teletype:complete", prog)
			return
		}

		if status == StatusPaused {
			s.mu.Lock()
			prog := s.snapshot()
			s.mu.Unlock()
			app.Event.Emit("teletype:progress", prog)
			time.Sleep(200 * time.Millisecond)
			continue
		}

		typeCharacter(rune(text[idx]))

		s.mu.Lock()
		s.currentIndex++
		newIdx := s.currentIndex
		var prog Progress
		if newIdx%10 == 0 || newIdx == total {
			prog = s.snapshot()
		}
		s.mu.Unlock()

		saveOffset(offsetPath, newIdx)

		if newIdx%10 == 0 || newIdx == total {
			app.Event.Emit("teletype:progress", prog)
		}

		time.Sleep(50 * time.Millisecond)
	}
}
