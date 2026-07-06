package prekey

import (
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
)

func VerifySignedPrekeySignature(identityKey ed25519.PublicKey, keyID uint32, prekeyPub, signature []byte) error {
	if len(identityKey) != ed25519.PublicKeySize {
		return errors.New("prekey: invalid identity key length")
	}
	if len(prekeyPub) != ed25519.PublicKeySize {
		return errors.New("prekey: invalid prekey public key length")
	}
	if len(signature) != ed25519.SignatureSize {
		return errors.New("prekey: invalid signature length")
	}
	payload := make([]byte, 4+len(prekeyPub))
	binary.BigEndian.PutUint32(payload[:4], keyID)
	copy(payload[4:], prekeyPub)
	if !ed25519.Verify(identityKey, payload, signature) {
		return errors.New("prekey: signature invalid")
	}
	return nil
}

func VerifyBundle(identityKey ed25519.PublicKey, b *Bundle) error {
	if err := VerifySignedPrekeySignature(identityKey, b.SPK.KeyID, b.SPK.PublicKey, b.SPK.Signature); err != nil {
		return fmt.Errorf("SPK: %w", err)
	}
	if b.OTK != nil {
		if err := VerifySignedPrekeySignature(identityKey, b.OTK.KeyID, b.OTK.PublicKey, b.OTK.Signature); err != nil {
			return fmt.Errorf("OTK: %w", err)
		}
	}
	return nil
}

func ContainsID(ids []uint64, target uint64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
