package main

func prepareStorage() (StorageHandler, StorageHandler, *RedisClusterStorageManager, *RPCStorageHandler, *RPCStorageHandler) {
	var tokenStore, orgStore StorageHandler

	tokenStore = &RedisClusterStorageManager{KeyPrefix: "apikey-", HashKeys: config.HashKeys}
	orgStore = &RedisClusterStorageManager{KeyPrefix: "orgkey."}
	healthStore := &RedisClusterStorageManager{KeyPrefix: "apihealth."}
	rpcAuthStore := &RPCStorageHandler{KeyPrefix: "apikey-", HashKeys: config.HashKeys, UserKey: config.SlaveOptions.APIKey, Address: config.SlaveOptions.ConnectionString}
	rpcOrgStore := &RPCStorageHandler{KeyPrefix: "orgkey.", UserKey: config.SlaveOptions.APIKey, Address: config.SlaveOptions.ConnectionString}

	if config.EnableEmbeddedKV {
		tokenStore = &DistributedKVStore{KeyPrefix: "apikey-", HashKeys: config.HashKeys}
		orgStore = &DistributedKVStore{KeyPrefix: "orgkey."}
	}

	FallbackKeySesionManager.Init(tokenStore)

	return tokenStore, orgStore, healthStore, rpcAuthStore, rpcOrgStore
}

func GetGlobalLocalStorageHandler(keyPrefix string, hashKeys bool) StorageHandler {
	if config.EnableEmbeddedKV {
		return &DistributedKVStore{KeyPrefix: keyPrefix, HashKeys: hashKeys}
	}

	return &RedisClusterStorageManager{KeyPrefix: keyPrefix, HashKeys: hashKeys}
}

func GetGlobalLocalCacheStorageHandler(keyPrefix string, hashKeys bool) StorageHandler {
	return &RedisClusterStorageManager{KeyPrefix: keyPrefix, HashKeys: hashKeys, IsCache: true}
}

func GetGlobalStorageHandler(keyPrefix string, hashKeys bool) StorageHandler {
	if config.SlaveOptions.UseRPC {
		return &RPCStorageHandler{KeyPrefix: keyPrefix, HashKeys: hashKeys, UserKey: config.SlaveOptions.APIKey, Address: config.SlaveOptions.ConnectionString}
	}

	if config.EnableEmbeddedKV {
		return &DistributedKVStore{KeyPrefix: keyPrefix, HashKeys: hashKeys}
	}

	return &RedisClusterStorageManager{KeyPrefix: keyPrefix, HashKeys: hashKeys}
}
