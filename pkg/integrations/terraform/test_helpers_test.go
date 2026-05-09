package terraform

import "context"

type recordingEncryptor struct {
	encryptAD []byte
	decryptAD []byte
}

func (e *recordingEncryptor) Encrypt(ctx context.Context, plaintext, associatedData []byte) ([]byte, error) {
	e.encryptAD = append([]byte(nil), associatedData...)
	return append([]byte(nil), plaintext...), nil
}

func (e *recordingEncryptor) Decrypt(ctx context.Context, ciphertext, associatedData []byte) ([]byte, error) {
	e.decryptAD = append([]byte(nil), associatedData...)
	return append([]byte(nil), ciphertext...), nil
}

func int64Ptr(v int64) *int64 {
	return &v
}
