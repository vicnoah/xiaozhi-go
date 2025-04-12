#include <opus/opus.h>
#include <portaudio.h>
#include <stdlib.h>
#include <stdio.h>

// 导出Opus函数
int wrapper_opus_encoder_create(opus_int32 Fs, int channels, int application, int *error)
{
    OpusEncoder *encoder = opus_encoder_create(Fs, channels, application, error);
    return (int)(intptr_t)encoder;
}

int wrapper_opus_encoder_destroy(int encoder)
{
    opus_encoder_destroy((OpusEncoder*)(intptr_t)encoder);
    return 0;
}

int wrapper_opus_encode(int encoder, const opus_int16 *pcm, int frame_size, unsigned char *data, int max_data_bytes)
{
    return opus_encode((OpusEncoder*)(intptr_t)encoder, pcm, frame_size, data, max_data_bytes);
}

// 导出Opus解码器函数
int wrapper_opus_decoder_create(opus_int32 Fs, int channels, int *error)
{
    OpusDecoder *decoder = opus_decoder_create(Fs, channels, error);
    return (int)(intptr_t)decoder;
}

int wrapper_opus_decoder_destroy(int decoder)
{
    opus_decoder_destroy((OpusDecoder*)(intptr_t)decoder);
    return 0;
}

int wrapper_opus_decode(int decoder, const unsigned char *data, int len, opus_int16 *pcm, int frame_size, int decode_fec)
{
    return opus_decode((OpusDecoder*)(intptr_t)decoder, data, len, pcm, frame_size, decode_fec);
}

// 导出Opus其他函数
const char* wrapper_opus_get_version_string(void)
{
    return opus_get_version_string();
}

// 导出PortAudio函数
int wrapper_Pa_Initialize(void)
{
    return Pa_Initialize();
}

int wrapper_Pa_Terminate(void)
{
    return Pa_Terminate();
}

int wrapper_Pa_GetDeviceCount(void)
{
    return Pa_GetDeviceCount();
}

const PaDeviceInfo* wrapper_Pa_GetDeviceInfo(int device)
{
    return Pa_GetDeviceInfo(device);
}

int wrapper_Pa_OpenStream(int *stream, int inputDevice, int outputDevice, double sampleRate,
                          unsigned long framesPerBuffer, unsigned long streamFlags)
{
    PaStream **s = (PaStream**)stream;
    PaStreamParameters inputParams, outputParams;
    PaStreamParameters *inputParamsPtr = NULL;
    PaStreamParameters *outputParamsPtr = NULL;
    
    if (inputDevice >= 0) {
        inputParams.device = inputDevice;
        inputParams.channelCount = 1;  // 单声道
        inputParams.sampleFormat = paInt16;
        inputParams.suggestedLatency = Pa_GetDeviceInfo(inputDevice)->defaultLowInputLatency;
        inputParams.hostApiSpecificStreamInfo = NULL;
        inputParamsPtr = &inputParams;
    }
    
    if (outputDevice >= 0) {
        outputParams.device = outputDevice;
        outputParams.channelCount = 1;  // 单声道
        outputParams.sampleFormat = paInt16;
        outputParams.suggestedLatency = Pa_GetDeviceInfo(outputDevice)->defaultLowOutputLatency;
        outputParams.hostApiSpecificStreamInfo = NULL;
        outputParamsPtr = &outputParams;
    }
    
    return Pa_OpenStream(s, inputParamsPtr, outputParamsPtr, sampleRate, framesPerBuffer, 
                         streamFlags, NULL, NULL);
}

int wrapper_Pa_StartStream(int stream)
{
    return Pa_StartStream((PaStream*)(intptr_t)stream);
}

int wrapper_Pa_StopStream(int stream)
{
    return Pa_StopStream((PaStream*)(intptr_t)stream);
}

int wrapper_Pa_CloseStream(int stream)
{
    return Pa_CloseStream((PaStream*)(intptr_t)stream);
}

int wrapper_Pa_IsStreamActive(int stream)
{
    return Pa_IsStreamActive((PaStream*)(intptr_t)stream);
}

const char* wrapper_Pa_GetErrorText(int errorCode)
{
    return Pa_GetErrorText(errorCode);
}

int wrapper_Pa_ReadStream(int stream, void *buffer, unsigned long frames)
{
    return Pa_ReadStream((PaStream*)(intptr_t)stream, buffer, frames);
}

int wrapper_Pa_WriteStream(int stream, const void *buffer, unsigned long frames)
{
    return Pa_WriteStream((PaStream*)(intptr_t)stream, buffer, frames);
}

// 测试函数
int main() {
    printf("Opus版本: %s\n", wrapper_opus_get_version_string());
    printf("PortAudio初始化: %d\n", wrapper_Pa_Initialize());
    printf("PortAudio设备数量: %d\n", wrapper_Pa_GetDeviceCount());
    wrapper_Pa_Terminate();
    return 0;
} 