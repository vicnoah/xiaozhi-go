package audio

import (
	"github.com/justa-cai/go-libopus/opus"
)

// Encoder 音频编码器接口
type Encoder interface {
	// Encode 将PCM数据编码为压缩格式
	Encode(pcmData []int16) ([]byte, error)
}

// Decoder 音频解码器接口
type Decoder interface {
	// Decode 将压缩格式解码为PCM数据
	Decode(compressedData []byte, pcmData []int16) (int, error)
}

// OpusCodec 实现Opus编解码
type OpusCodec struct {
	encoder *opus.OpusEncoder
	decoder *opus.OpusDecoder
	buffer  []byte
}

// NewOpusCodec 创建新的Opus编解码器
func NewOpusCodec(sampleRate, channelCount int) (*OpusCodec, error) {
	// 创建Opus编码器
	encoder, err := opus.NewEncoder(sampleRate, channelCount, opus.OpusApplicationAudio)
	if err != nil {
		return nil, err
	}

	// 创建Opus解码器
	decoder, err := opus.NewDecoder(sampleRate, channelCount)
	if err != nil {
		return nil, err
	}

	return &OpusCodec{
		encoder: encoder,
		decoder: decoder,
		buffer:  make([]byte, 1024), // 参考 go-libopus 示例
	}, nil
}

// Encode 将PCM数据编码为Opus格式
func (c *OpusCodec) Encode(pcmData []int16) ([]byte, error) {
	// go-libopus 需要输入 []byte，需转换
	input := make([]byte, len(pcmData)*2)
	for i, v := range pcmData {
		input[2*i] = byte(v)
		input[2*i+1] = byte(v >> 8)
	}
	n, err := c.encoder.Encode(input, c.buffer)
	if err != nil {
		return nil, err
	}
	result := make([]byte, n)
	copy(result, c.buffer[:n])
	return result, nil
}

// Decode 将Opus格式解码为PCM数据
func (c *OpusCodec) Decode(opusData []byte, pcmData []int16) (int, error) {
	output := make([]byte, len(pcmData)*2)
	nSamples, err := c.decoder.Decode(opusData, output)
	if err != nil {
		return 0, err
	}
	// []byte 转回 []int16
	for i := 0; i < nSamples*2 && i/2 < len(pcmData); i += 2 {
		pcmData[i/2] = int16(output[i]) | int16(output[i+1])<<8
	}
	return nSamples, nil
}

// Close 关闭编解码器并释放资源
func (c *OpusCodec) Close() {
	c.encoder.Close()
	c.decoder.Close()
	c.encoder = nil
	c.decoder = nil
}
