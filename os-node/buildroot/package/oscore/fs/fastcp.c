/* fastcp - Fast file copy using optimized sendfile */
#include "fs.h"
#include <stdio.h>
#include <string.h>
#include <time.h>

void print_usage(const char *prog) {
    printf("Usage: %s <source> <destination>\n", prog);
    printf("Fast file copy using zero-copy sendfile optimization\n");
}

int main(int argc, char *argv[]) {
    if (argc != 3) {
        print_usage(argv[0]);
        return 1;
    }

    const char *src = argv[1];
    const char *dst = argv[2];

    printf("Copying %s -> %s ...\n", src, dst);

    struct timespec start, end;
    clock_gettime(CLOCK_MONOTONIC, &start);

    ssize_t bytes = fs_sendfile(src, dst);

    clock_gettime(CLOCK_MONOTONIC, &end);

    if (bytes < 0) {
        perror("Error copying file");
        return 1;
    }

    double elapsed = (end.tv_sec - start.tv_sec) + 
                     (end.tv_nsec - start.tv_nsec) / 1000000000.0;

    printf("Copied %zd bytes in %.3f seconds (%.2f MB/s)\n",
           bytes, elapsed, (bytes / 1024.0 / 1024.0) / elapsed);

    return 0;
}
