package bkv

import "errors"

var (
	// ErrChecksumMismatch checksum校验失败
	ErrChecksumMismatch = errors.New("checksum mismatch")
)

// CalculateChecksum 计算BKV协议校验和
// BKV校验和算法：对数据区所有字节进行累加（byte溢出自动丢弃高位）
// 数据区：从len字段开始，到校验和字节之前（不包含magic和tail）
func CalculateChecksum(data []byte) byte {
	var checksum byte
	for _, b := range data {
		checksum += b
	}
	return checksum
}

// VerifyChecksum 验证校验和
// dataWithChecksum: 包含校验和的完整数据（从 len字段到校验和）
func VerifyChecksum(dataWithChecksum []byte) error {
	if len(dataWithChecksum) < 1 {
		return errors.New("data too short for checksum verification")
	}

	// 最后一个字节是校验和
	checksumPos := len(dataWithChecksum) - 1
	receivedChecksum := dataWithChecksum[checksumPos]

	// 计算预期的校验和（不包含校验和字节本身）
	expectedChecksum := CalculateChecksum(dataWithChecksum[:checksumPos])

	if receivedChecksum != expectedChecksum {
		return ErrChecksumMismatch
	}

	return nil
}

// BuildChecksummedData 为数据添加校验和
// data: 不包含校验和的数据（从 len字段到数据部分）
// 返回：带校验和的完整数据
func BuildChecksummedData(data []byte) []byte {
	checksum := CalculateChecksum(data)
	result := make([]byte, len(data)+1)
	copy(result, data)
	result[len(data)] = checksum
	return result
}
