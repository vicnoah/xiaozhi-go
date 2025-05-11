//go:build darwin

package audio

/*
#cgo LDFLAGS: -framework CoreAudio -framework AudioUnit -framework AudioToolbox
#include <CoreAudio/CoreAudio.h>
#include <AudioUnit/AudioUnit.h>
#include <AudioToolbox/AudioToolbox.h>
#include <stdlib.h>

// 这里只做骨架，建议后续用go-mac/coreaudio或cgo补全
*/
import "C"
import (
	"errors"
	"sync"
)

type darwinRecorder struct {
	isRecording bool
	onAudioData func([]byte)
	onPCMData   func([]int16, int)
	stopCh      chan struct{}
	mu          sync.Mutex
}

func newRecorder() Recorder {
	return &darwinRecorder{}
}

func (r *darwinRecorder) StartRecording(codec Encoder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRecording {
		return errors.New("录音已在进行中")
	}
	// TODO: 这里需要用CoreAudio API实现音频采集
	r.isRecording = true
	r.stopCh = make(chan struct{})
	// 伪实现：直接返回未实现
	go func() {
		// 你可以在这里实现CoreAudio采集并回调
	}()
	return errors.New("macOS录音功能未实现")
}

func (r *darwinRecorder) StopRecording() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRecording {
		return nil
	}
	close(r.stopCh)
	r.isRecording = false
	return nil
}
func (r *darwinRecorder) Close() error {
	return r.StopRecording()
}
func (r *darwinRecorder) SetAudioDataCallback(cb func([]byte)) {
	r.onAudioData = cb
}
func (r *darwinRecorder) SetPCMDataCallback(cb func([]int16, int)) {
	r.onPCMData = cb
}
func (r *darwinRecorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRecording
}
