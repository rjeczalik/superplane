package registry_test

import (
	"bytes"
	"crypto"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
	"github.com/glebarez/sqlite"
	"github.com/superplanehq/superplane/pkg/integrations/terraform/registry"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestKeyPinStoreResolveKey(t *testing.T) {
	source, err := registry.ParseProviderSource("registry.terraform.io/siderolabs/talos")
	if err != nil {
		t.Fatal(err)
	}
	hashicorpSource, err := registry.ParseProviderSource("registry.terraform.io/hashicorp/aws")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("production without preconfigured key rejects", func(t *testing.T) {
		store := newKeyPinStore(t)
		_, err := store.ResolveKey(source, openpgp.EntityList{newTestEntity(t)}, registry.KeyPinPolicy{Production: true})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("explicit TOFU gate pins key and emits audit event", func(t *testing.T) {
		store := newKeyPinStore(t)
		key := newTestEntity(t)
		var events []registry.KeyPinAuditEvent

		keys, err := store.ResolveKey(source, openpgp.EntityList{key}, registry.KeyPinPolicy{
			Production: true,
			AllowTOFU:  true,
			Audit: func(event registry.KeyPinAuditEvent) {
				events = append(events, event)
			},
		})
		if err != nil {
			t.Fatalf("ResolveKey() error = %v", err)
		}
		if len(keys) != 1 || len(events) != 1 {
			t.Fatalf("expected key and audit event, got keys=%d events=%d", len(keys), len(events))
		}
	})

	t.Run("second download with same fingerprint succeeds", func(t *testing.T) {
		store := newKeyPinStore(t)
		key := newTestEntity(t)
		_, err := store.ResolveKey(source, openpgp.EntityList{key}, registry.KeyPinPolicy{AllowTOFU: true})
		if err != nil {
			t.Fatalf("initial ResolveKey() error = %v", err)
		}

		if _, err := store.ResolveKey(source, openpgp.EntityList{key}, registry.KeyPinPolicy{Production: true}); err != nil {
			t.Fatalf("ResolveKey() error = %v", err)
		}
	})

	t.Run("second download with different fingerprint rejects", func(t *testing.T) {
		store := newKeyPinStore(t)
		key := newTestEntity(t)
		_, err := store.ResolveKey(source, openpgp.EntityList{key}, registry.KeyPinPolicy{AllowTOFU: true})
		if err != nil {
			t.Fatalf("initial ResolveKey() error = %v", err)
		}

		if _, err := store.ResolveKey(source, openpgp.EntityList{newTestEntity(t)}, registry.KeyPinPolicy{Production: true}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("operator preconfigured fingerprint takes precedence", func(t *testing.T) {
		store := newKeyPinStore(t)
		key := newTestEntity(t)
		fingerprint := registry.FingerprintForEntity(key)

		keys, err := store.ResolveKey(source, openpgp.EntityList{key}, registry.KeyPinPolicy{
			Production:           true,
			OperatorFingerprints: []string{strings.ToLower(fingerprint)},
		})
		if err != nil {
			t.Fatalf("ResolveKey() error = %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("expected one key, got %d", len(keys))
		}
	})

	t.Run("HashiCorp provider skips TOFU and uses built-in key", func(t *testing.T) {
		store := newKeyPinStore(t)
		key := newTestEntity(t)

		keys, err := store.ResolveKey(hashicorpSource, openpgp.EntityList{key}, registry.KeyPinPolicy{Production: true})
		if err != nil {
			t.Fatalf("ResolveKey() error = %v", err)
		}
		if len(keys) != 1 {
			t.Fatalf("expected one key, got %d", len(keys))
		}
	})
}

func newKeyPinStore(t *testing.T) *registry.KeyPinStore {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE terraform_provider_gpg_keys (
  id text PRIMARY KEY,
  provider_source text NOT NULL,
  key_id text NOT NULL,
  fingerprint text NOT NULL,
  ascii_armor text NOT NULL,
  trust_mode text NOT NULL,
  pinned_at datetime NOT NULL
)`).Error; err != nil {
		t.Fatalf("create terraform_provider_gpg_keys: %v", err)
	}
	if err := db.Exec(`CREATE UNIQUE INDEX idx_tf_provider_gpg_keys_source ON terraform_provider_gpg_keys (provider_source)`).Error; err != nil {
		t.Fatalf("create terraform_provider_gpg_keys source index: %v", err)
	}

	return registry.NewKeyPinStore(db)
}

func armoredPublicKey(t *testing.T, entity *openpgp.Entity) string {
	t.Helper()

	var out bytes.Buffer
	writer, err := armor.Encode(&out, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor encode: %v", err)
	}
	if err := entity.Serialize(writer); err != nil {
		t.Fatalf("serialize entity: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close armor: %v", err)
	}

	return out.String()
}

func TestParseArmoredKey(t *testing.T) {
	entity := newTestEntity(t)
	armored := armoredPublicKey(t, entity)

	keys, err := registry.ParseArmoredKey([]byte(armored))
	if err != nil {
		t.Fatalf("ParseArmoredKey() error = %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected one key, got %d", len(keys))
	}
}

func newTestEntityWithName(t *testing.T, name string) *openpgp.Entity {
	t.Helper()

	entity, err := openpgp.NewEntity(name, "test", name+"@example.com", &packet.Config{
		DefaultHash: crypto.SHA256,
	})
	if err != nil {
		t.Fatalf("NewEntity() error = %v", err)
	}

	return entity
}
