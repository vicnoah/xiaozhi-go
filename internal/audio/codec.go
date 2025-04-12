package audio

import (
	"github.com/hraban/opus"
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
	encoder *opus.Encoder
	decoder *opus.Decoder
	buffer  []byte
}

// NewOpusCodec 创建新的Opus编解码器
func NewOpusCodec(sampleRate, channelCount int) (*OpusCodec, error) {
	// 创建Opus编码器
	encoder, err := opus.NewEncoder(sampleRate, channelCount, opus.AppVoIP)
	if err != nil {
		return nil, err
	}

	// 创建Opus解码器
	decoder, err := opus.NewDecoder(sampleRate, channelCount)
	if err != nil {
		return nil, err
	}

	// 设置编码器参数
	if err := encoder.SetBitrate(16000); err != nil {
		return nil, err
	}
	if err := encoder.SetComplexity(10); err != nil {
		return nil, err
	}

	return &OpusCodec{
		encoder: encoder,
		decoder: decoder,
		buffer:  make([]byte, 1000), // 足够存储编码后的Opus帧
	}, nil
}

// Encode 将PCM数据编码为Opus格式
func (c *OpusCodec) Encode(pcmData []int16) ([]byte, error) {
	n, err := c.encoder.Encode(pcmData, c.buffer)
	if err != nil {
		return nil, err
	}

	// 复制结果以避免后续操作影响缓冲区
	result := make([]byte, n)
	copy(result, c.buffer[:n])

	return result, nil
}

// Decode 将Opus格式解码为PCM数据
func (c *OpusCodec) Decode(opusData []byte, pcmData []int16) (int, error) {
	return c.decoder.Decode(opusData, pcmData)
}

// Close 关闭编解码器并释放资源
func (c *OpusCodec) Close() {
	// opus库没有提供显式的关闭方法，不需要特别的清理操作
	c.encoder = nil
	c.decoder = nil
}
