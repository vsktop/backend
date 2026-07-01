package prekey

import (
	"crypto/ed25519"
	"crypto/subtle"
	"errors"
)

func VerifySignedPrekeySignature(identityKey ed25519.PublicKey, prekeyPub, signature []byte) error {
	if len(identityKey) != ed25519.PublicKeySize {
		return errors.New("prekey: invalid identity key length")
	}
	if len(prekeyPub) != ed25519.PublicKeySize {
		return errors.New("prekey: invalid prekey public key length")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("prekey: invalid signature length")
	}

	if !ed25519.Verify(identityKey, prekeyPub, signature) {
		return errors.New("prekey: signed prekey signature invalid under account identity key")
	}
	return nil
}

func VerifyPrekeyBundle(identityKey ed25519.PublicKey, signedPrekeyPub, signedPrekeySig []byte) error {
	if err := VerifySignedPrekeySignature(identityKey, signedPrekeyPub, signedPrekeySig); err != nil {
		return err
	}
	return nil
}

func ContainsID(constant []uint64, target uint64) bool {
	targetBytes := make([]byte, 8)
	for i := range targetBytes {
		targetBytes[i] = byte(target >> (i * 8))
	}
	for _, id := range constant {
		idBytes := make([]byte, 8)
		for i := range idBytes {
			idBytes[i] = byte(id >> (i * 8))
		}
		if subtle.ConstantTimeCompare(idBytes, targetBytes) == 1 {
			return true
		}
	}
	return false
}
