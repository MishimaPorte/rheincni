#include <stdio.h>
#include <stdlib.h>
#include <assert.h>
#include <string.h>
#include <errno.h>
#include <unistd.h>
#include <stdint.h>
#include <fcntl.h>
#include <net/if.h>
#include <linux/veth.h>
#include <linux/rtnetlink.h>
#include <sys/ioctl.h>
#include <sys/socket.h>
#include <sys/syscall.h>

#include "lib/types.h"
#define REQ_SIZE 1024

static struct rtattr *add_attr(struct nlmsghdr *nlm,
                               size_t maxlen,
                               uint16_t type,
                               const void *data,
                               size_t data_len)
{
    size_t len     = RTA_LENGTH(data_len);
    size_t offset  = NLMSG_ALIGN(nlm->nlmsg_len);
    size_t new_len = offset + RTA_ALIGN(len);

    if (new_len > maxlen) {
        errno = EMSGSIZE;
        return NULL;
    }

    struct rtattr *rta = (struct rtattr *)((void *)nlm + offset);
    rta->rta_type = type;
    rta->rta_len  = len;
    if (data_len) memcpy(RTA_DATA(rta), data, data_len);

    nlm->nlmsg_len = new_len;
    return rta;
}

static void end_nested(struct nlmsghdr *nlh,
                       struct rtattr *rta)
{
    void *end   = (void *)nlh + NLMSG_ALIGN(nlh->nlmsg_len);
    void *start = (void*)rta;
    rta->rta_len = (uint16_t)(end - start);
}

int get_errno()
{
    return errno;
}


int set_interface_up(const char *name)
{
    struct ifreq ifr = {0};
    if (strlen(name) >= IFNAMSIZ)
        return -1;

    memcpy(ifr.ifr_name, name, strlen(name) + 1);

    int fd = socket(AF_INET, SOCK_DGRAM, 0);
    if (fd < 0)
        return -1;

    int rc = ioctl(fd, SIOCGIFFLAGS, &ifr);
    if (rc == 0) {
        ifr.ifr_flags |= IFF_UP;
        rc = ioctl(fd, SIOCSIFFLAGS, &ifr);
    }

    int saved = errno;
    close(fd);
    errno = saved;
    return rc;
}


int create_veth_peer(const char *host_name,
                     const char *peer_name,
                     int netns_fd,
                     mac *host_mac,
                     mac *peer_mac)
{
    size_t host_len = strlen(host_name);
    size_t peer_len = strlen(peer_name);
    if (host_len >= IFNAMSIZ || peer_len >= IFNAMSIZ) {
        errno = ENAMETOOLONG;
        return -1;
    }

    struct {
        struct nlmsghdr nlh;
        struct ifinfomsg ifi;
        char attributes[REQ_SIZE];
    } req = {
        .nlh = {
            .nlmsg_len   = NLMSG_LENGTH(sizeof(struct ifinfomsg)),
            .nlmsg_type  = RTM_NEWLINK,
            .nlmsg_flags = NLM_F_REQUEST |
                           NLM_F_CREATE  |
                           NLM_F_EXCL    |
                           NLM_F_ACK,
            .nlmsg_seq   = 1,
        },
        .ifi = {
            .ifi_flags = IFF_UP,
            .ifi_family = AF_UNSPEC,
        },
    };

    if (!add_attr(&req.nlh, sizeof req, IFLA_IFNAME, host_name, host_len + 1))
        return -1;
    if (!add_attr(&req.nlh, sizeof req, IFLA_ADDRESS, host_mac, sizeof *host_mac))
        return -1;

    struct rtattr *linkinfo = add_attr(&req.nlh, sizeof req, IFLA_LINKINFO, NULL, 0);
    if (!linkinfo)
        return -1;

    if (!add_attr(&req.nlh, sizeof req, IFLA_INFO_KIND, "veth", sizeof "veth"))
        return -1;


    struct rtattr *infodata = add_attr(&req.nlh, sizeof req, IFLA_INFO_DATA, NULL, 0);
    if (!infodata)
        return -1;

    struct ifinfomsg peer_ifi = {
        .ifi_family = AF_UNSPEC,
    };

    struct rtattr *peer = add_attr(&req.nlh, sizeof req, VETH_INFO_PEER, &peer_ifi, sizeof peer_ifi);
    if (!peer)
        return -1;

    if (!add_attr(&req.nlh, sizeof req, IFLA_IFNAME, peer_name, peer_len + 1))
        return -1;

    if (!add_attr(&req.nlh, sizeof req, IFLA_ADDRESS, peer_mac, sizeof *peer_mac))
        return -1;

    if (!add_attr(&req.nlh, sizeof req, IFLA_NET_NS_FD, &netns_fd, sizeof netns_fd))
        return -1;

    end_nested(&req.nlh, peer);
    end_nested(&req.nlh, infodata);
    end_nested(&req.nlh, linkinfo);

    int fd = socket(AF_NETLINK, SOCK_RAW, NETLINK_ROUTE);
    if (fd < 0)
        return -1;

    struct sockaddr_nl kernel = {
        .nl_family = AF_NETLINK,
    };

    if (sendto(fd, &req, req.nlh.nlmsg_len, 0, (struct sockaddr *)&kernel, sizeof kernel) < 0) {
        int saved = errno;
        close(fd);
        errno = saved;
        return -1;
    }

    char msg[1024 * 1024];

    ssize_t received = recv(fd, &msg, sizeof msg, 0);
    // struct nlmsghdr *recv_h = (struct nlmsghdr *)&msg[received];
    // received += rx;
    // if (received >= recv_h->nlmsg_len) {
    //     printf("first packet received: %zu -> %zu\n", received, recv_h->nlmsg_len);
    //     if (recv_h->nlmsg_type == NLMSG_ERROR) {
    //         printf("type: %zu\n", recv_h->nlmsg_type);
    //         struct nlmsgerr *error = (void*)(recv_h + 1);
    //         errno = -error->error;
    //         return -1;
    //     }
    //
    //     if (recv_h->nlmsg_type != RTM_NEWLINK) {
    //         errno = EPROTO;
    //         return -1;
    //     }
    //     struct nlmsghdr *recv_h2 = (struct nlmsghdr *)&msg[recv_h->nlmsg_len];
    //     if (received < recv_h->nlmsg_len + recv_h2->nlmsg_len)
    //         while (received < recv_h->nlmsg_len + recv_h2->nlmsg_len)
    //             received += recv(fd, &msg[received], sizeof msg - received, 0);
    //
    // } else {
    //     while (received < recv_h->nlmsg_len)
    //         received += recv(fd, &msg[received], sizeof msg - received, 0);
    //     struct nlmsghdr *recv_h2 = (struct nlmsghdr *)&msg[recv_h->nlmsg_len];
    //     if (received <= recv_h->nlmsg_len + recv_h2->nlmsg_len)
    //         while (received < recv_h->nlmsg_len + recv_h2->nlmsg_len)
    //             received += recv(fd, &msg[received], sizeof msg - received, 0);
    // }
    if (!received) {
        int saved = errno;
        close(fd);
        errno = saved;
        return -1;
    }

    struct nlmsghdr *h = (struct nlmsghdr *)&msg[0];
    if (h->nlmsg_type != NLMSG_ERROR) {
        errno = EPROTO;
        return -1;
    }
    struct nlmsgerr *error = (void*)(h + 1);
    if (error->error) {
        errno = -error->error;
        return -1;
    }

    int oldns = open("/proc/self/ns/net", O_RDONLY);
    if (oldns < 0)
        return -1;

    int rc = syscall(__NR_setns, netns_fd, 0);
    if (rc < 0)
        return -1;
    if (set_interface_up(peer_name)) return -1;
    rc = syscall(__NR_setns, oldns, 0);
    if (rc < 0)
        return -1;
    return 0;
}

