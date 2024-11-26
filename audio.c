#include <stdint.h>
#include "_cgo_export.h"
#include <stdio.h>

void cOnAudio(uintptr_t userdata, unsigned char *stream, int len)
{
	// fprintf(stderr, "calling Go OnAudio func\n");
	OnAudio(userdata, stream, len);
}

