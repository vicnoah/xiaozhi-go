//go:build ignore

package main

// #cgo CFLAGS: -I${SRCDIR}/../mingw64/include -I${SRCDIR}/../mingw64/include/opus
// #cgo LDFLAGS: -L${SRCDIR}/../mingw64/lib -lopus -lportaudio -lwinmm
// #include ".github/windows_opus_wrapper.c"
import "C"

import (
	"fmt"
	"unsafe"
)

// OpusEncoder 表示Opus编码器
type OpusEncoder struct {
	ptr C.int
}

// OpusDecoder 表示Opus解码器
type OpusDecoder struct {
	ptr C.int
}

// NewOpusEncoder 创建新的Opus编码器
func NewOpusEncoder(sampleRate, channels, application int) (*OpusEncoder, error) {
	var err C.int
	encoderPtr := C.wrapper_opus_encoder_create(C.opus_int32(sampleRate), C.int(channels), C.int(application), &err)
	if err != 0 {
		return nil, fmt.Errorf("创建编码器失败: %d", err)
	}
	return &OpusEncoder{ptr: encoderPtr}, nil
}

// Encode 编码PCM数据
func (e *OpusEncoder) Encode(pcm []int16, frameSize int, data []byte) (int, error) {
	if len(pcm) == 0 || len(data) == 0 {
		return 0, fmt.Errorf("无效的输入数据")
	}

	pcmPtr := (*C.opus_int16)(unsafe.Pointer(&pcm[0]))
	dataPtr := (*C.uchar)(unsafe.Pointer(&data[0]))

	encoded := C.wrapper_opus_encode(e.ptr, pcmPtr, C.int(frameSize), dataPtr, C.int(len(data)))
	if encoded < 0 {
		return 0, fmt.Errorf("编码失败: %d", encoded)
	}
	return int(encoded), nil
}

// Close 关闭编码器
func (e *OpusEncoder) Close() {
	C.wrapper_opus_encoder_destroy(e.ptr)
}

// NewOpusDecoder 创建新的Opus解码器
func NewOpusDecoder(sampleRate, channels int) (*OpusDecoder, error) {
	var err C.int
	decoderPtr := C.wrapper_opus_decoder_create(C.opus_int32(sampleRate), C.int(channels), &err)
	if err != 0 {
		return nil, fmt.Errorf("创建解码器失败: %d", err)
	}
	return &OpusDecoder{ptr: decoderPtr}, nil
}

// Decode 解码Opus数据
func (d *OpusDecoder) Decode(data []byte, pcm []int16, frameSize int, decodeFec bool) (int, error) {
	if len(data) == 0 || len(pcm) == 0 {
		return 0, fmt.Errorf("无效的输入数据")
	}

	dataPtr := (*C.uchar)(unsafe.Pointer(&data[0]))
	pcmPtr := (*C.opus_int16)(unsafe.Pointer(&pcm[0]))

	decodeFecInt := 0
	if decodeFec {
		decodeFecInt = 1
	}

	decoded := C.wrapper_opus_decode(d.ptr, dataPtr, C.int(len(data)), pcmPtr, C.int(frameSize), C.int(decodeFecInt))
	if decoded < 0 {
		return 0, fmt.Errorf("解码失败: %d", decoded)
	}
	return int(decoded), nil
}

// Close 关闭解码器
func (d *OpusDecoder) Close() {
	C.wrapper_opus_decoder_destroy(d.ptr)
}

// GetOpusVersion 获取Opus版本
func GetOpusVersion() string {
	return C.GoString(C.wrapper_opus_get_version_string())
}

// 以下为主函数，用于测试
func main() {
	fmt.Println("Opus版本:", GetOpusVersion())
	fmt.Println("初始化PortAudio:", C.wrapper_Pa_Initialize())
	fmt.Println("设备数量:", C.wrapper_Pa_GetDeviceCount())
	C.wrapper_Pa_Terminate()
}
