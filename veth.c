#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>
#include <fcntl.h>
#include <net/if.h>
#include <linux/if_tun.h>
#include <sys/ioctl.h>
#include <sys/socket.h>

#include "helper.h"

int veth_create_device(int tun_fd, const char *name, const char **err)
{
    struct ifreq ifr = {0};
    assert(name && "device name to be created is not provided");

    ifr.ifr_flags = IFF_TAP | IFF_NO_PI;
    strncpy(ifr.ifr_name, name, IFNAMSIZ);

   // Tell the kernel to create the interface
    if (ioctl(tun_fd, TUNSETIFF, (void *)&ifr) < 0) {
        *err = temp_sprintf("failed to create a tup devide: %s", strerror(errno));
        return -1;
    }
}
