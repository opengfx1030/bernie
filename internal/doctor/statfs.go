package doctor

import "syscall"

func statfsFree(p string) uint64 {
	var st syscall.Statfs_t
	if p == "" || syscall.Statfs(p, &st) != nil {
		return 0
	}
	return uint64(st.Bavail) * uint64(st.Bsize)
}
