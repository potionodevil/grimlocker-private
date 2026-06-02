#ifndef GRIMLOCKER_C_H
#define GRIMLOCKER_C_H
#include <stdint.h>
#include <stddef.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct GrimlockerClient GrimlockerClient;

GrimlockerClient* grimlocker_client_new(const char* base_url, const char* token);
void grimlocker_client_free(GrimlockerClient* client);

// Auth
int grimlocker_unlock_vault(GrimlockerClient* c, const char* password);
int grimlocker_lock_vault(GrimlockerClient* c);

// Entries
char* grimlocker_list_entries(GrimlockerClient* c, const char* category);
char* grimlocker_get_entry(GrimlockerClient* c, const char* id);
char* grimlocker_create_entry(GrimlockerClient* c, const char* title, const char* category);
int grimlocker_update_entry(GrimlockerClient* c, const char* id, const char* title);
int grimlocker_delete_entry(GrimlockerClient* c, const char* id);
char* grimlocker_search_entries(GrimlockerClient* c, const char* query, const char* category);

// File Vault
int grimlocker_upload_file(GrimlockerClient* c, const uint8_t* data, size_t len,
    const char* filename, const char* mime_type, const char* folder_id,
    void (*on_progress)(size_t sent, size_t total));
int grimlocker_download_file(GrimlockerClient* c, const char* manifest_block_id,
    uint8_t** out_data, size_t* out_len);

// Workspaces
char* grimlocker_list_workspaces(GrimlockerClient* c);
char* grimlocker_create_workspace(GrimlockerClient* c, const char* name);

// Sync
char* grimlocker_list_sync_peers(GrimlockerClient* c);
int grimlocker_trigger_sync(GrimlockerClient* c);

// Audit
char* grimlocker_list_audit_events(GrimlockerClient* c, int n);

// Health
char* grimlocker_health_check(GrimlockerClient* c);

// Memory: caller must free returned strings with grimlocker_free_string()
void grimlocker_free_string(char* s);
void grimlocker_free_data(uint8_t* data, size_t len);

// Last error
const char* grimlocker_last_error(GrimlockerClient* c);

#ifdef __cplusplus
}
#endif
#endif
