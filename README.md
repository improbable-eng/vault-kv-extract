![vault-kv-extract](https://github.com/improbable/vault-kv-extract/blob/master/images/black-and-white-dark-keys.jpg)

# vault-kv-extract

This repo holds the script we used to migrate hidden Vault (v0.6.5) secrets out of an etcd v3 storage backend. Read more about our migration escapade at *TODO(keeley): Add link to Breaking Into Our Own Vault of Secrets blog post.*

## Example usage

### 1. Snapshot etcd storage backend

```shell
$ # Exec into GKE node with Vault
$ kubectl exec -it $ETCD_NODE_NAME -- /bin/sh
/# etcdctl --version                                                                                  
etcdctl version: 3.3.2
API version: 2
$ # Snapshot etcd keyspace
/# ETCDCTL_API=3 etcdctl --endpoints $ENDPOINT snapshot save snapshot.db
$ # Copy snapshot from GKE node to local machine
$ kubectl cp $ETCD_NODE_NAME:snapshot.db /tmp/etcd_backup
```

### 2. Restore snapshot to a local etcd cluster

```shell
$ ETCDCTL_API=3 etcdctl snapshot restore /tmp/etcd_backup/snapshot.db \
--name m1 \
--initial-cluster m1=http://localhost:2380 \
--initial-cluster-token etcd-cluster-1 \
--initial-advertise-peer-urls http://localhost:2380
```

### 3. Start local etcd cluster

```shell
$  etcd --version                                                                                      
etcd Version: 3.3.2
Git SHA: GitNotFound
Go Version: go1.10
Go OS/Arch: darwin/amd64
$ cd /tmp/etcd_backup && etcd \
--name m1 \
--listen-client-urls http://localhost:2379 \
--advertise-client-urls http://localhost:2379 \
--listen-peer-urls http://localhost:2380
```

### 4. Get keys for Vault secrets

```shell
$ ETCDCTL_API=3 etcdctl get / --prefix --keys-only
/vault/logical/$UUID/$PATH_TO_KEY
...
```

### 5. Get the project

```shell
$ go get github.com/improbable-eng/vault-kv-extract
```

### 6. Migrate a secret

To migrate the secret at `/vault/logical/$UUID/$PATH_TO_KEY` to `/secret/$PATH_TO_KEY` in the destination Vault

```shell
$ vault-kv-extract \
--origin_vault_backend_name "logical/$UUID" \
--destination_vault_backend_name "secret/" \
--origin_vault_master_key_shares "$SHARE1 $SHARE2 $SHARE$" \
--origin_vault_keys_paths $PATH_TO_KEY \
--destination_vault_address $VAULT_ADDR \
--destination_vault_token $VAULT_TOKEN
```
