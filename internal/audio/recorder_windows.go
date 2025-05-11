//go:build windows

package audio

/*
#cgo LDFLAGS: -lwinmm
#include <windows.h>
#include <mmsystem.h>
#include <stdlib.h>

HWAVEIN hWaveIn;
WAVEHDR waveHdr;
short *buffer;

int start_recording(int sampleRate, int channels, int bufsize) {
    WAVEFORMATEX wfx;
    wfx.wFormatTag = WAVE_FORMAT_PCM;
    wfx.nChannels = channels;
    wfx.nSamplesPerSec = sampleRate;
    wfx.wBitsPerSample = 16;
    wfx.nBlockAlign = wfx.nChannels * wfx.wBitsPerSample / 8;
    wfx.nAvgBytesPerSec = wfx.nSamplesPerSec * wfx.nBlockAlign;
    wfx.cbSize = 0;

    buffer = (short*)malloc(bufsize * sizeof(short));
    if (waveInOpen(&hWaveIn, WAVE_MAPPER, &wfx, 0, 0, CALLBACK_NULL) != MMSYSERR_NOERROR) {
        return -1;
    }
    waveHdr.lpData = (LPSTR)buffer;
    waveHdr.dwBufferLength = bufsize * sizeof(short);
    waveHdr.dwFlags = 0;
    waveHdr.dwLoops = 0;
    if (waveInPrepareHeader(hWaveIn, &waveHdr, sizeof(WAVEHDR)) != MMSYSERR_NOERROR) {
        return -2;
    }
    if (waveInAddBuffer(hWaveIn, &waveHdr, sizeof(WAVEHDR)) != MMSYSERR_NOERROR) {
        return -3;
    }
    if (waveInStart(hWaveIn) != MMSYSERR_NOERROR) {
        return -4;
    }
    return 0;
}
int read_pcm(int bufsize) {
    if (waveHdr.dwFlags & WHDR_DONE) {
        return bufsize;
    }
    return 0;
}
void stop_recording() {
    waveInStop(hWaveIn);
    waveInReset(hWaveIn);
    waveInUnprepareHeader(hWaveIn, &waveHdr, sizeof(WAVEHDR));
    waveInClose(hWaveIn);
    free(buffer);
}
*/
import "C"
import (
	"errors"
	"sync"
	"time"
	"unsafe"
)

type winRecorder struct {
	isRecording bool
	onAudioData func([]byte)
	onPCMData   func([]int16, int)
	stopCh      chan struct{}
	mu          sync.Mutex
}

func newRecorder() Recorder {
	return &winRecorder{}
}

func (r *winRecorder) StartRecording(codec Encoder) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.isRecording {
		return errors.New("录音已在进行中")
	}
	sampleRate := 16000
	channels := 1
	framesPerBuffer := 960 // 60ms at 16kHz

	if C.start_recording(C.int(sampleRate), C.int(channels), C.int(framesPerBuffer)) != 0 {
		return errors.New("打开Windows录音设备失败")
	}
	r.isRecording = true
	r.stopCh = make(chan struct{})

	go func() {
		for {
			select {
			case <-r.stopCh:
				return
			default:
			}
			n := C.read_pcm(C.int(framesPerBuffer))
			if int(n) > 0 {
				// 取出C.buffer
				buf := (*[1 << 20]C.short)(unsafe.Pointer(C.buffer))[:int(n)]
				// 回调PCM数据
				if r.onPCMData != nil {
					pcm := make([]int16, int(n))
					for i := 0; i < int(n); i++ {
						pcm[i] = int16(buf[i])
					}
					r.onPCMData(pcm, int(n))
				}
				// 回调原始字节数据
				if r.onAudioData != nil {
					b := make([]byte, int(n)*2)
					for i := 0; i < int(n); i++ {
						b[2*i] = byte(buf[i])
						b[2*i+1] = byte(buf[i] >> 8)
					}
					r.onAudioData(b)
				}
				time.Sleep(60 * time.Millisecond)
			} else {
				time.Sleep(10 * time.Millisecond)
			}
		}
	}()
	return nil
}

func (r *winRecorder) StopRecording() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.isRecording {
		return nil
	}
	close(r.stopCh)
	r.isRecording = false
	C.stop_recording()
	return nil
}
func (r *winRecorder) Close() error {
	return r.StopRecording()
}
func (r *winRecorder) SetAudioDataCallback(cb func([]byte)) {
	r.onAudioData = cb
}
func (r *winRecorder) SetPCMDataCallback(cb func([]int16, int)) {
	r.onPCMData = cb
}
func (r *winRecorder) IsRecording() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.isRecording
}
