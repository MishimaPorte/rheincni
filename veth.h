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

#include "lib/types.h"

int create_veth_peer(const char *host_name,
                     const char *peer_name,
                     int netns_fd,
                     mac *host_mac,
                     mac *peer_mac,
                     int *new_index);
int get_errno();
