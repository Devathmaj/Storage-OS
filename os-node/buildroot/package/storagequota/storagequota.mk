################################################################################
#
# storagequota
#
################################################################################

STORAGEQUOTA_VERSION = 1.0
STORAGEQUOTA_SITE_METHOD = local
STORAGEQUOTA_SITE = $(BR2_EXTERNAL_STORAGEOS_PATH)/../package/storagequota

define STORAGEQUOTA_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/quotad.sh $(TARGET_DIR)/usr/sbin/quotad
	$(INSTALL) -D -m 0755 $(@D)/quotactl.sh $(TARGET_DIR)/usr/bin/quotactl
	$(INSTALL) -D -m 0755 $(@D)/S85quotad $(TARGET_DIR)/etc/init.d/S85quotad
endef

$(eval $(generic-package))
