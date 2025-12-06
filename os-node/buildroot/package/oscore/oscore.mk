################################################################################
#
# oscore - Optimized File Storage
#
################################################################################

OSCORE_VERSION = 1.0.0
OSCORE_SITE = $(TOPDIR)/package/oscore
OSCORE_SITE_METHOD = local
OSCORE_INSTALL_STAGING = YES
OSCORE_INSTALL_TARGET = YES
OSCORE_DEPENDENCIES = cloudflared

define OSCORE_BUILD_CMDS
	$(MAKE) CC="$(TARGET_CC)" LD="$(TARGET_LD)" -C $(@D)/fs all
	$(MAKE) CC="$(TARGET_CC)" LD="$(TARGET_LD)" -C $(@D)/server all
	$(MAKE) CC="$(TARGET_CC)" LD="$(TARGET_LD)" -C $(@D)/metadata_loader all
endef

define OSCORE_INSTALL_STAGING_CMDS
	$(INSTALL) -D -m 0644 $(@D)/fs/libfs.a $(STAGING_DIR)/usr/lib/libfs.a
	$(INSTALL) -D -m 0755 $(@D)/fs/libfs.so $(STAGING_DIR)/usr/lib/libfs.so
	$(INSTALL) -D -m 0755 $(@D)/fs/libfs_preload.so $(STAGING_DIR)/usr/lib/libfs_preload.so
	$(INSTALL) -D -m 0644 $(@D)/fs/fs.h $(STAGING_DIR)/usr/include/fs.h
endef

define OSCORE_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/fs/libfs.so $(TARGET_DIR)/usr/lib/libfs.so
	$(INSTALL) -D -m 0755 $(@D)/fs/libfs_preload.so $(TARGET_DIR)/usr/lib/libfs_preload.so
	$(INSTALL) -D -m 0644 $(@D)/fs/fs.h $(TARGET_DIR)/usr/include/fs.h
	$(INSTALL) -D -m 0644 $(@D)/fs/99-fs-optimize.conf $(TARGET_DIR)/etc/ld.so.preload.d/99-fs-optimize.conf
	$(INSTALL) -D -m 0755 $(@D)/fs/test_preload $(TARGET_DIR)/usr/bin/test_preload
	$(INSTALL) -D -m 0755 $(@D)/server/storageos-httpd $(TARGET_DIR)/usr/sbin/storageos-httpd
	$(INSTALL) -D -m 0755 $(@D)/server/S95httpd $(TARGET_DIR)/etc/init.d/S95httpd
	$(INSTALL) -D -m 0755 $(@D)/metadata_loader/metadata_loader $(TARGET_DIR)/usr/bin/metadata_loader
	$(INSTALL) -D -m 0755 $(@D)/storage/S96osnode $(TARGET_DIR)/etc/init.d/S96osnode
	$(INSTALL) -D -m 0755 $(@D)/metadata_loader/S97metadata $(TARGET_DIR)/etc/init.d/S97metadata
	$(INSTALL) -D -m 0755 $(@D)/fs/S99fsoptimize $(TARGET_DIR)/etc/init.d/S99fsoptimize
	$(INSTALL) -D -m 0755 $(@D)/storage/S90storage $(TARGET_DIR)/usr/bin/init-storage
	$(INSTALL) -d -m 0755 $(TARGET_DIR)/var/cache
	$(INSTALL) -D -m 0644 $(@D)/cloudflare/config.yml.template $(TARGET_DIR)/etc/cloudflared/config.yml.template
	$(INSTALL) -D -m 0755 $(@D)/cloudflare/setup_tunnel $(TARGET_DIR)/usr/bin/setup_tunnel
	$(INSTALL) -D -m 0755 $(@D)/cloudflare/setup_mtls $(TARGET_DIR)/usr/bin/setup_mtls
	$(INSTALL) -D -m 0755 $(@D)/cloudflare/call_server $(TARGET_DIR)/usr/bin/call_server
	$(INSTALL) -D -m 0755 $(@D)/cloudflare/auto_register $(TARGET_DIR)/usr/bin/auto_register
	$(INSTALL) -D -m 0755 $(@D)/cloudflare/S98cloudflared $(TARGET_DIR)/etc/init.d/S98cloudflared
	# Copy enrollment and node management tools from controller build
	$(INSTALL) -D -m 0755 $(TOPDIR)/../../controller/bin/os-storage-enroll $(TARGET_DIR)/usr/bin/os-storage-enroll
	$(INSTALL) -D -m 0755 $(TOPDIR)/../../controller/bin/os-remove-node $(TARGET_DIR)/usr/bin/os-remove-node
endef

$(eval $(generic-package))
