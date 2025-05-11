package audio

// 这里已移除portaudio相关内容，如需录音请用oto库实现。

type Recorder interface {
	StartRecording(codec Encoder) error
	StopRecording() error
	Close() error
	SetAudioDataCallback(cb func([]byte))
	SetPCMDataCallback(cb func([]int16, int))
	IsRecording() bool
}

// NewRecorder 返回当前平台的录音器实例
func NewRecorder() Recorder {
	return newRecorder()
}
