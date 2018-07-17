# 封包格式

[x][x][x][x] | [x][x] | [x][x] | [x]... | [x]...
---|---|---|---|---
(uint32) | (uint16) | (uint16) | (binary) | (binary)
4-byte | 2-byte | 2-byte | N-byte | N-byte
length | extendSize | frameType | data | extend

包头固定长度为8个字节，前4字节为包的长度，5-6字节为路由的长度，7-8字节为事件，data为包的内容，extend为路由的内容。
封包的处理非按接受到的顺序，需求对顺序敏感的，请另处理。