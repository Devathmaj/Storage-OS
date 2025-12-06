################################################################################
#
# storagemgr - Interactive Storage Manager Package
#
################################################################################

STORAGEMGR_VERSION = 1.0
STORAGEMGR_SITE = $(TOPDIR)/package/storagemgr
STORAGEMGR_SITE_METHOD = local
STORAGEMGR_LICENSE = MIT

define STORAGEMGR_INSTALL_TARGET_CMDS
	$(INSTALL) -D -m 0755 $(@D)/storagemgr-v3.sh $(TARGET_DIR)/usr/bin/storagemgr
endef

$(eval $(generic-package))
