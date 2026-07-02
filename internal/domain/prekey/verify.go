package prekey

import (
	"crypto/ed25519"
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
	return VerifySignedPrekeySignature(identityKey, signedPrekeyPub, signedPrekeySig)
}

func ContainsID(ids []uint64, target uint64) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
