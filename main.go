// Copyright (c) Improbable Worlds Ltd, All Rights Reserved

package main

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/physical"
	"github.com/hashicorp/vault/vault"
	"github.com/pkg/errors"
	"github.com/hashicorp/vault/shamir"
)

var (
	fDestinationVaultAddress = flag.String(
		"destination_vault_address",
		"",
		"The address of the Vault to migrate data to",
	)
	fDestinationVaultBackendName = flag.String(
		"destination_vault_backend_name",
		"",
		"Name of the backend in the destination Vault in which to place migrated data, e.g. secret/",
	)
	fDestinationVaultToken = flag.String(
		"destination_vault_token",
		"",
		"A token with full write permission to the destination Vault",
	)
	fOriginVaultKeysPaths = flag.String(
		"origin_vault_keys_paths",
		"",
		"Paths to the physical locations of the storage keys in the encrypted backend to migrate",
	)
	fOriginVaultBackendName = flag.String(
		"origin_vault_backend_name",
		"",
		"Name of the backend in the origin Vault in which the data to migrate is stored",
	)
	fOriginVaultMasterKeyShares = flag.String(
		"origin_vault_master_key_shares",
		"",
		"Space-delimited shares, at least the minimum number of shares necessary to reconstruct the Vault master key, e.g. 'k1 k2 k3'",
	)
)

// Example:
//
// $ ETCDCTL_API=3 etcdctl get / --prefix --keys-only
//   /vault/logical/$UUID/$PATH_TO_KEY
//   ...
//
// Flags to migrate the secret at /vault/logical/$UUID/$PATH_TO_KEY to /secret/$PATH_TO_KEY
//
// --origin_vault_keys_paths=$PATH_TO_KEY
// --origin_vault_backend_name=/vault/logical/$UUID
// --destination_vault_backend_name=secret/

func extract() (map[string][]byte, error) {
	// The backend config must match the backend of the Vault instance from which data is being migrated.
	b, err := physical.NewBackend("etcd", nil, map[string]string{
		"address":    "http://127.0.0.1:2379",
		"ha_enabled": "true",
		"etcd_api":   "v3",
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to initialize new etcd backend")
	}

	barrier, err := vault.NewAESGCMBarrier(b)
	if err != nil {
		return nil, errors.Wrap(err, "failed to instantiate new barrier")
	}

	k, err := reconstructMasterKey()
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconstruct master key")
	}

	if err := barrier.Unseal(k); err != nil {
		return nil, errors.Wrap(err, "failed to unseal etcd backend")
	}

	secrets := map[string][]byte{}
	paths := strings.Split(*fOriginVaultKeysPaths, " ")
	if len(paths) == 0 {
		return nil, errors.New("no paths to Vault keys specified")
	}
	for _, p := range paths {
		backendName := normalizeBackendName(*fOriginVaultBackendName)
		path := strings.TrimPrefix(p, "/")
		ent, err := barrier.Get(backendName + path)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get value for key at: %v", p)
		}
		if ent == nil {
			return nil, errors.New("no entry found for key")
		}
		secrets[strings.TrimPrefix(ent.Key, backendName)] = ent.Value
	}

	return secrets, nil
}

func migrate(secrets map[string][]byte) error {
	if *fDestinationVaultAddress == "" {
		return errors.New("no destination Vault address set")
	}
	cl, err := api.NewClient(&api.Config{
		Address: *fDestinationVaultAddress,
	})
	if err != nil {
		return err
	}
	if *fDestinationVaultToken == "" {
		return errors.New("no destination Vault token set")
	}
	cl.SetToken(*fDestinationVaultToken)

	for k, v := range secrets {
		var secret map[string]interface{}
		if err := json.Unmarshal(v, &secret); err != nil {
			return err
		}
		backendName := normalizeBackendName(*fDestinationVaultBackendName)
		if _, err := cl.Logical().Write(backendName + k, secret); err != nil {
			return err
		}
		fmt.Printf("wrote key %v\n", k)
	}

	return nil
}

func reconstructMasterKey() ([]byte, error) {
	rawShares := strings.Split(*fOriginVaultMasterKeyShares, " ")
	var shares [][]byte
	for _, s := range rawShares {
		share, err := base64.StdEncoding.DecodeString(s)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode share: %v", s)
		}
		shares = append(shares, share)
	}

	return shamir.Combine(shares)
}

func normalizeBackendName(backendName string) string {
	s := strings.TrimPrefix(backendName, "/")
	if !strings.HasSuffix(backendName, "/") {
		s += "/"
	}

	return s // desired format e.g. logical/
}

func main() {
	flag.Parse()

	secrets, err := extract()
	if err != nil {
		panic(err)
	}

	if err := migrate(secrets); err != nil {
		panic(err)
	}
}
