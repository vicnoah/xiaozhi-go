#!/bin/bash

# 此脚本用于在Windows环境下创建opus和portaudio的pkg-config文件
# 为Windows构建过程解决pkg-config找不到库文件的问题

# 确保目录存在
mkdir -p "/c/pkgconfig"

# 生成opus.pc文件
cat > "/c/pkgconfig/opus.pc" << EOF
prefix=/c/msys64/mingw64
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: opus
Description: Opus IETF audio codec
Version: 1.3.1
Libs: -L\${libdir} -lopus
Cflags: -I\${includedir}/opus
EOF

# 生成opusfile.pc文件
cat > "/c/pkgconfig/opusfile.pc" << EOF
prefix=/c/msys64/mingw64
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: opusfile
Description: High-level Opus decoding library
Version: 0.12
Requires.private: opus >= 1.0.1, ogg >= 1.3
Libs: -L\${libdir} -lopusfile
Cflags: -I\${includedir}/opus
EOF

# 生成portaudio-2.0.pc文件
cat > "/c/pkgconfig/portaudio-2.0.pc" << EOF
prefix=/c/msys64/mingw64
exec_prefix=\${prefix}
libdir=\${exec_prefix}/lib
includedir=\${prefix}/include

Name: portaudio-2.0
Description: Portable audio I/O
Version: 19
Libs: -L\${libdir} -lportaudio
Cflags: -I\${includedir}
EOF

echo "已创建pkg-config文件在 /c/pkgconfig/"
ls -la /c/pkgconfig/ 