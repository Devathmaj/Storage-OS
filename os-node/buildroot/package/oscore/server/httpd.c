/*
 * HTTP/TCP Server for StorageOS
 * Lightweight HTTP server for API communication
 */

#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <sys/socket.h>
#include <sys/types.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <signal.h>
#include <errno.h>
#include <time.h>
#include <sys/select.h>
#include <fcntl.h>

#define DEFAULT_PORT 8080
#define BACKLOG 128
#define BUFFER_SIZE 8192
#define MAX_CLIENTS 64

static volatile int running = 1;

typedef struct {
    int fd;
    struct sockaddr_in addr;
    time_t last_activity;
    char buffer[BUFFER_SIZE];
    size_t buffer_len;
} client_t;

void signal_handler(int sig) {
    if (sig == SIGINT || sig == SIGTERM) {
        printf("\nShutting down server...\n");
        running = 0;
    }
}

int set_nonblocking(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags == -1) return -1;
    return fcntl(fd, F_SETFL, flags | O_NONBLOCK);
}

void send_http_response(int client_fd, int status_code, const char *status_text, 
                       const char *content_type, const char *body) {
    char header[1024];
    time_t now = time(NULL);
    struct tm *tm_info = gmtime(&now);
    char date_buf[128];
    
    strftime(date_buf, sizeof(date_buf), "%a, %d %b %Y %H:%M:%S GMT", tm_info);
    
    int body_len = body ? strlen(body) : 0;
    
    snprintf(header, sizeof(header),
             "HTTP/1.1 %d %s\r\n"
             "Server: StorageOS/1.0\r\n"
             "Date: %s\r\n"
             "Content-Type: %s\r\n"
             "Content-Length: %d\r\n"
             "Connection: keep-alive\r\n"
             "\r\n",
             status_code, status_text, date_buf, content_type, body_len);
    
    send(client_fd, header, strlen(header), 0);
    if (body && body_len > 0) {
        send(client_fd, body, body_len, 0);
    }
}

void handle_http_request(client_t *client) {
    char method[16], path[256], version[16];
    
    // Parse first line: METHOD PATH HTTP/VERSION
    if (sscanf(client->buffer, "%15s %255s %15s", method, path, version) != 3) {
        send_http_response(client->fd, 400, "Bad Request", 
                          "text/plain", "Invalid HTTP request");
        return;
    }
    
    printf("[%s] %s %s\n", inet_ntoa(client->addr.sin_addr), method, path);
    
    // Handle different endpoints
    if (strcmp(path, "/") == 0 || strcmp(path, "/health") == 0) {
        char response[512];
        snprintf(response, sizeof(response),
                 "{\"status\":\"ok\",\"service\":\"StorageOS\",\"version\":\"1.0.0\"}");
        send_http_response(client->fd, 200, "OK", "application/json", response);
    }
    else if (strcmp(path, "/api/status") == 0) {
        char response[512];
        snprintf(response, sizeof(response),
                 "{\"status\":\"running\",\"uptime\":%ld,\"connections\":\"active\"}",
                 time(NULL));
        send_http_response(client->fd, 200, "OK", "application/json", response);
    }
    else if (strncmp(path, "/api/", 5) == 0) {
        // API endpoint placeholder
        char response[512];
        snprintf(response, sizeof(response),
                 "{\"message\":\"API endpoint ready\",\"path\":\"%s\",\"method\":\"%s\"}",
                 path, method);
        send_http_response(client->fd, 200, "OK", "application/json", response);
    }
    else {
        send_http_response(client->fd, 404, "Not Found", 
                          "text/plain", "Endpoint not found");
    }
}

int main(int argc, char *argv[]) {
    int server_fd, port = DEFAULT_PORT;
    struct sockaddr_in server_addr;
    client_t clients[MAX_CLIENTS] = {0};
    fd_set read_fds;
    int max_fd;
    
    // Parse arguments
    if (argc > 1) {
        port = atoi(argv[1]);
        if (port <= 0 || port > 65535) {
            fprintf(stderr, "Invalid port number\n");
            return 1;
        }
    }
    
    // Setup signal handlers
    signal(SIGINT, signal_handler);
    signal(SIGTERM, signal_handler);
    signal(SIGPIPE, SIG_IGN);
    
    // Create socket
    server_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (server_fd < 0) {
        perror("socket");
        return 1;
    }
    
    // Set socket options
    int opt = 1;
    if (setsockopt(server_fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt)) < 0) {
        perror("setsockopt");
        close(server_fd);
        return 1;
    }
    
    // Make socket non-blocking
    if (set_nonblocking(server_fd) < 0) {
        perror("set_nonblocking");
        close(server_fd);
        return 1;
    }
    
    // Bind socket
    memset(&server_addr, 0, sizeof(server_addr));
    server_addr.sin_family = AF_INET;
    server_addr.sin_addr.s_addr = INADDR_ANY;
    server_addr.sin_port = htons(port);
    
    if (bind(server_fd, (struct sockaddr *)&server_addr, sizeof(server_addr)) < 0) {
        perror("bind");
        close(server_fd);
        return 1;
    }
    
    // Listen
    if (listen(server_fd, BACKLOG) < 0) {
        perror("listen");
        close(server_fd);
        return 1;
    }
    
    printf("StorageOS HTTP Server started on port %d\n", port);
    printf("Ready to accept API connections...\n");
    
    // Initialize client array
    for (int i = 0; i < MAX_CLIENTS; i++) {
        clients[i].fd = -1;
    }
    
    // Main server loop
    while (running) {
        FD_ZERO(&read_fds);
        FD_SET(server_fd, &read_fds);
        max_fd = server_fd;
        
        // Add active clients to fd_set
        for (int i = 0; i < MAX_CLIENTS; i++) {
            if (clients[i].fd > 0) {
                FD_SET(clients[i].fd, &read_fds);
                if (clients[i].fd > max_fd) {
                    max_fd = clients[i].fd;
                }
            }
        }
        
        // Wait for activity
        struct timeval timeout = {1, 0};  // 1 second timeout
        int activity = select(max_fd + 1, &read_fds, NULL, NULL, &timeout);
        
        if (activity < 0 && errno != EINTR) {
            perror("select");
            break;
        }
        
        if (activity == 0) continue;  // Timeout
        
        // Check for new connections
        if (FD_ISSET(server_fd, &read_fds)) {
            struct sockaddr_in client_addr;
            socklen_t addr_len = sizeof(client_addr);
            int client_fd = accept(server_fd, (struct sockaddr *)&client_addr, &addr_len);
            
            if (client_fd >= 0) {
                // Find free slot for new client
                int slot = -1;
                for (int i = 0; i < MAX_CLIENTS; i++) {
                    if (clients[i].fd < 0) {
                        slot = i;
                        break;
                    }
                }
                
                if (slot >= 0) {
                    set_nonblocking(client_fd);
                    clients[slot].fd = client_fd;
                    clients[slot].addr = client_addr;
                    clients[slot].last_activity = time(NULL);
                    clients[slot].buffer_len = 0;
                    printf("New connection from %s:%d (slot %d)\n",
                           inet_ntoa(client_addr.sin_addr),
                           ntohs(client_addr.sin_port), slot);
                } else {
                    printf("Max clients reached, rejecting connection\n");
                    close(client_fd);
                }
            }
        }
        
        // Check existing clients
        for (int i = 0; i < MAX_CLIENTS; i++) {
            if (clients[i].fd < 0) continue;
            
            if (FD_ISSET(clients[i].fd, &read_fds)) {
                char temp_buf[BUFFER_SIZE];
                ssize_t bytes = recv(clients[i].fd, temp_buf, sizeof(temp_buf) - 1, 0);
                
                if (bytes <= 0) {
                    // Connection closed or error
                    printf("Client disconnected (slot %d)\n", i);
                    close(clients[i].fd);
                    clients[i].fd = -1;
                    continue;
                }
                
                temp_buf[bytes] = '\0';
                
                // Append to buffer
                if (clients[i].buffer_len + bytes < BUFFER_SIZE - 1) {
                    memcpy(clients[i].buffer + clients[i].buffer_len, temp_buf, bytes);
                    clients[i].buffer_len += bytes;
                    clients[i].buffer[clients[i].buffer_len] = '\0';
                }
                
                // Check for complete HTTP request (ends with \r\n\r\n)
                if (strstr(clients[i].buffer, "\r\n\r\n")) {
                    handle_http_request(&clients[i]);
                    // Reset buffer for next request (keep-alive)
                    clients[i].buffer_len = 0;
                    clients[i].buffer[0] = '\0';
                }
                
                clients[i].last_activity = time(NULL);
            }
        }
        
        // Cleanup idle connections (60 second timeout)
        time_t now = time(NULL);
        for (int i = 0; i < MAX_CLIENTS; i++) {
            if (clients[i].fd > 0 && (now - clients[i].last_activity) > 60) {
                printf("Closing idle connection (slot %d)\n", i);
                close(clients[i].fd);
                clients[i].fd = -1;
            }
        }
    }
    
    // Cleanup
    printf("Closing all connections...\n");
    for (int i = 0; i < MAX_CLIENTS; i++) {
        if (clients[i].fd > 0) {
            close(clients[i].fd);
        }
    }
    close(server_fd);
    
    printf("Server stopped\n");
    return 0;
}
