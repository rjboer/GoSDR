Binary opcodes (supported by your responder)
Opcode (hex)	Name (from header)	Meaning (high-level)
0x00	RESPONSE	Generic response / status for requests
0x01	PRINT	Server-side informational/diagnostic message
0x02	TIMEOUT	Server indicates timeout occurred
0x03	READ_ATTR	Read device attribute
0x04	READ_DBG_ATTR	Read debug attribute
0x05	READ_BUF_ATTR	Read buffer attribute
0x06	READ_CHN_ATTR	Read channel attribute
0x07	WRITE_ATTR	Write device attribute
0x08	WRITE_DBG_ATTR	Write debug attribute
0x09	WRITE_BUF_ATTR	Write buffer attribute
0x0A	WRITE_CHN_ATTR	Write channel attribute
0x0B	GETTRIG	Read trigger configuration
0x0C	SETTRIG	Set trigger configuration
0x0D	CREATE_BUFFER	Allocate/create a buffer object
0x0E	FREE_BUFFER	Free a buffer object
0x0F	ENABLE_BUFFER	Enable/start a buffer
0x10	DISABLE_BUFFER	Disable/stop a buffer
0x11	CREATE_BLOCK	Allocate/create a transfer block for a buffer
0x12	FREE_BLOCK	Free a transfer block
0x13	TRANSFER_BLOCK	Transfer a block payload (RX/TX data movement)
0x14	ENQUEUE_BLOCK_CYCLIC	Mark/enqueue a block as cyclic (commonly TX replay)
0x15	RETRY_DEQUEUE_BLOCK	Retry dequeue operation (flow-control / scheduling)
0x16	CREATE_EVSTREAM	Create an event stream
0x17	FREE_EVSTREAM	Free an event stream
0x18	READ_EVENT	Read one event from the event stream