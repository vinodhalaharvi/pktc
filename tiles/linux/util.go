package linux

import "strconv"

func u32(v uint32) string { return strconv.FormatUint(uint64(v), 10) }

func u16(v uint16) string { return strconv.FormatUint(uint64(v), 10) }
