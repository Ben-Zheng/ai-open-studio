package utils

import "math/rand"

const letterBytes = "abcdefghijklmnopqrstuvwxyz"
const inviteLeterBytes = "23456789abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ"

const (
	letterIdxBits = 6                    // 6 bits to represent a letter index
	letterIdxMask = 1<<letterIdxBits - 1 // All 1-bits, as many as letterIdxBits
	letterIdxMax  = 63 / letterIdxBits   // # of letter indices fitting in 63 bits
)

// RandStringBytesMaskImprByLetters 生成随机密码
func RandStringBytesMaskImprByLetters(letters string, n int) []byte {
	b := make([]byte, n)
	// A rand.Int63() generates 63 random bits, enough for letterIdxMax letters!
	for i, cache, remain := n-1, rand.Int63(), letterIdxMax; i >= 0; {
		if remain == 0 {
			cache, remain = rand.Int63(), letterIdxMax
		}

		if idx := int(cache & letterIdxMask); idx < len(letters) {
			b[i] = letters[idx]
			i--
		}

		cache >>= letterIdxBits
		remain--
	}

	return b
}

// RandStringBytesMaskImpr 生成随机密码
func RandStringBytesMaskImpr(n int) []byte {
	return RandStringBytesMaskImprByLetters(letterBytes, n)
}
