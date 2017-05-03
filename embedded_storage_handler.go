package main

import (
	"encoding/json"
	"strings"

	"github.com/TykTechnologies/tyk-cluster-framework/client"
	"github.com/TykTechnologies/tyk-cluster-framework/distributed_store"
	"github.com/TykTechnologies/tyk-cluster-framework/distributed_store/rafty"
	"github.com/TykTechnologies/tyk-cluster-framework/encoding"
)

var KeyValueStore *DistributedKVStore

type DistributedKVStore struct {
	KeyPrefix   string
	HashKeys    bool
	StoreConfig *rafty.Config
	Store       *tcf.DistributedStore
	beacon      client.Client
}

func (d *DistributedKVStore) Connect() bool {
	if d.Store != nil {
		d.Store.Stop()
		d.beacon.Stop()
	}

	var err error

	if d.beacon, err = client.NewClient("beacon://0.0.0.0:9999?interval=1", encoding.JSON); err != nil {
		log.Fatal("Could not create a beacon: ", err)
		return false
	}

	d.Store, err = tcf.NewDistributedStore(d.StoreConfig)
	if err != nil {
		log.Fatal("Failed to create a new distributed store: ", err)
		return false
	}

	d.Store.Start("", d.beacon)
	return true
}

func (d *DistributedKVStore) getKey(k string) (string, error) {
	v, err := d.Store.StorageAPI.GetKey(k)
	if err != nil {
		return "", err
	}

	return v.Node.Value, nil
}

func (d *DistributedKVStore) GetKey(k string) (string, error) {
	k = d.fixKey(k)
	return d.getKey(k)
}

func (d *DistributedKVStore) GetRawKey(k string) (string, error) {
	return d.getKey(k)
}

func (d *DistributedKVStore) setKey(k string, v string, ttl int64) error {
	_, err := d.Store.StorageAPI.UpdateKey(k, v, int(ttl))
	if strings.Contains(err.Error(), "Not found") {
		if _, err := d.Store.StorageAPI.CreateKey(k, v, int(ttl)); err != nil {
			return err
		}
	}

	return nil
}

func (d *DistributedKVStore) SetKey(k string, v string, ttl int64) error {
	k = d.fixKey(k)
	return d.setKey(k, v, ttl)
}

func (d *DistributedKVStore) SetRawKey(k string, v string, ttl int64) error {
	return d.setKey(k, v, ttl)
}

func (d *DistributedKVStore) deleteKey(k string) bool {
	_, err := d.Store.StorageAPI.DeleteKey(k)
	if err != nil {
		log.Error("Delete failed: ", err)
		return false
	}

	return true
}

func (d *DistributedKVStore) DeleteKey(k string) bool {
	k = d.fixKey(k)
	return d.deleteKey(k)
}

func (d *DistributedKVStore) DeleteRawKey(k string) bool {
	return d.deleteKey(k)
}

func (d *DistributedKVStore) GetSet(k string) (map[string]string, error) {
	k = d.fixKey(k)
	v, err := d.getKey(k)
	if err != nil {
		return nil, err
	}

	// Assume it's encoded properly
	var set map[string]string
	err = json.Unmarshal([]byte(v), &set)
	if err != nil {
		return nil, err
	}

	return set, nil
}

func (d *DistributedKVStore) AddToSet(k string, v string) {
	// no need to fix key, it's done in the next method
	set, err := d.GetSet(k)
	if err != nil {
		log.Error("Failed to add to set: ", err)
	}

	// This is to ensure only one instance of the object is ever stored, like a redis set
	set[v] = v

	j, err := json.Marshal(set)
	if err != nil {
		log.Error("Failed to encode back to set: ", err)
	}

	_, err = d.Store.StorageAPI.UpdateKey(k, string(j), 0)
	if err != nil {
		log.Error("Failed to update set: ", err)
	}
}

func (d *DistributedKVStore) RemoveFromSet(k string, v string) {
	k = d.fixKey(k)
	set, err := d.GetSet(k)
	if err != nil {
		log.Error("Failed to add to set: ", err)
	}

	// This is to ensure only one instance of the object is ever stored, like a redis set
	delete(set, v)

	j, err := json.Marshal(set)
	if err != nil {
		log.Error("Failed to encode back to set: ", err)
	}

	_, err = d.Store.StorageAPI.UpdateKey(k, string(j), 0)
	if err != nil {
		log.Error("Failed to update set: ", err)
	}
}

// No-ops, these can fail soft
func (d *DistributedKVStore) GetKeys(string) []string {
	log.Error("Key lists are not supported")
	return []string{}
}

func (d *DistributedKVStore) GetKeysAndValuesWithFilter(string) map[string]string {
	log.Error("Key/value lists are not supported")
	return map[string]string{}
}

func (d *DistributedKVStore) IncrememntWithExpire(string, int64) int64 {
	log.Error("IncrememntWithExpire is not supported with the distributed k/v store, please use the distributed rate limiter")
	return 0
}

func (d *DistributedKVStore) DeleteScanMatch(string) bool {
	log.Error("Please use an explicit redis cache, distributed k/v does not support scan matched deletes.")
	return false
}

// No-ops, these should fail hard
func (d *DistributedKVStore) SetRollingWindow(string, int64, string) (int, []interface{}) {
	log.Fatal("The Distributed store can only be used with the distributed rate limiter")
	return 0, nil
}
func (d *DistributedKVStore) SetRollingWindowPipeline(string, int64, string) (int, []interface{}) {
	log.Fatal("The Distributed store can only be used with the distributed rate limiter")
	return 0, nil
}

// Utility funcs to ensure consistent behaviour with the redis handlers
func (d *DistributedKVStore) hashKey(in string) string {
	if !d.HashKeys {
		// Not hashing? Return the raw key
		return in
	}
	return doHash(in)
}

func (d *DistributedKVStore) fixKey(keyName string) string {
	setKeyName := d.KeyPrefix + d.hashKey(keyName)

	log.Debug("Input key was: ", setKeyName)

	return setKeyName
}

func (d *DistributedKVStore) cleanKey(keyName string) string {
	setKeyName := strings.Replace(keyName, d.KeyPrefix, "", 1)
	return setKeyName
}
