#include "textflag.h"

TEXT ·ttPrefetch(SB), NOSPLIT, $0-8
	MOVQ addr+0(FP), AX
	PREFETCHT0 (AX)
	RET
