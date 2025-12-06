################################################################################
#
# os-storage-server - OS Storage Server with Dqlite
#
################################################################################

OS_STORAGE_SERVER_VERSION = 1.0
OS_STORAGE_SERVER_SITE = $(TOPDIR)/package/os-storage-server
OS_STORAGE_SERVER_SITE_METHOD = local
OS_STORAGE_SERVER_LICENSE = MIT
OS_STORAGE_SERVER_DEPENDENCIES = dqlite host-go
OS_STORAGE_SERVER_GOMOD = github.com/osstorage/server

define OS_STORAGE_SERVER_BUILD_CMDS
	cd $(@D) && \
	$(TARGET_MAKE_ENV) \
	GOPROXY=https://proxy.golang.org,direct \
	GOSUMDB=sum.golang.org \
	CGO_ENABLED=1 \
	CC=$(TARGET_CC) \
	CXX=$(TARGET_CXX) \
	GOOS=linux \
	GOARCH=$(GO_GOARCH) \
	CGO_CFLAGS="$(TARGET_CFLAGS) -I$(STAGING_DIR)/usr/include" \
	CGO_LDFLAGS="$(TARGET_LDFLAGS) -L$(STAGING_DIR)/usr/lib" \
	$(HOST_DIR)/bin/go build -o os-storage-server \
		-ldflags '-extldflags "-static-libgcc"' \
		./main.go && \
	$(HOST_DIR)/bin/go build -o os-storage-enroll \
		-ldflags '-extldflags "-static-libgcc"' \
		./enroll.go && \
	$(HOST_DIR)/bin/go build -o os-remove-node \
		-ldflags '-extldflags "-static-libgcc"' \
		./remove_node.go
endef

define OS_STORAGE_SERVER_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/os-storage-server $(TARGET_DIR)/usr/bin/os-storage-server
	$(INSTALL) -D -m 0755 $(@D)/os-storage-enroll $(TARGET_DIR)/usr/bin/os-storage-enroll
	$(INSTALL) -D -m 0755 $(@D)/os-remove-node $(TARGET_DIR)/usr/bin/os-remove-node
	$(INSTALL) -D -m 0755 $(@D)/server-connect $(TARGET_DIR)/usr/bin/server-connect
	$(INSTALL) -D -m 0755 $(@D)/S90osstorage $(TARGET_DIR)/etc/init.d/S90osstorage
	$(INSTALL) -d -m 0755 $(TARGET_DIR)/etc/osstorage
	$(INSTALL) -d -m 0755 $(TARGET_DIR)/etc/os
	$(INSTALL) -D -m 0644 $(@D)/config.yaml $(TARGET_DIR)/etc/osstorage/config.yaml
	$(INSTALL) -D -m 0644 $(@D)/motd $(TARGET_DIR)/etc/motd
endef

$(eval $(generic-package))
