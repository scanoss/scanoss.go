/* sample.c — deterministic fixture for WFP fingerprint golden/bench tests.
 * Do not edit: the committed sample.wfp.golden is derived from this exact file.
 */
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

#define MAX_NODES 1024
#define HASH_SEED 0x811c9dc5u

typedef struct node {
    int          key;
    char        *name;
    double       weight;
    struct node *next;
} node_t;

static unsigned int fnv1a(const char *data, size_t len) {
    unsigned int hash = HASH_SEED;
    for (size_t i = 0; i < len; i++) {
        hash ^= (unsigned char)data[i];
        hash *= 16777619u;
    }
    return hash;
}

static node_t *node_create(int key, const char *name, double weight) {
    node_t *n = (node_t *)malloc(sizeof(node_t));
    if (n == NULL) {
        return NULL;
    }
    n->key = key;
    n->name = strdup(name);
    n->weight = weight;
    n->next = NULL;
    return n;
}

static void node_free(node_t *n) {
    while (n != NULL) {
        node_t *next = n->next;
        free(n->name);
        free(n);
        n = next;
    }
}

static node_t *list_prepend(node_t *head, node_t *item) {
    item->next = head;
    return item;
}

static node_t *list_find(node_t *head, int key) {
    for (node_t *cur = head; cur != NULL; cur = cur->next) {
        if (cur->key == key) {
            return cur;
        }
    }
    return NULL;
}

static double list_total_weight(const node_t *head) {
    double total = 0.0;
    for (const node_t *cur = head; cur != NULL; cur = cur->next) {
        total += cur->weight;
    }
    return total;
}

static int compare_keys(const void *a, const void *b) {
    int ka = *(const int *)a;
    int kb = *(const int *)b;
    return (ka > kb) - (ka < kb);
}

int main(int argc, char **argv) {
    node_t *head = NULL;
    for (int i = 0; i < argc && i < MAX_NODES; i++) {
        unsigned int h = fnv1a(argv[i], strlen(argv[i]));
        node_t *n = node_create((int)(h % MAX_NODES), argv[i], (double)i * 1.5);
        if (n != NULL) {
            head = list_prepend(head, n);
        }
    }

    int keys[MAX_NODES];
    int count = 0;
    for (node_t *cur = head; cur != NULL && count < MAX_NODES; cur = cur->next) {
        keys[count++] = cur->key;
    }
    qsort(keys, (size_t)count, sizeof(int), compare_keys);

    printf("nodes=%d total_weight=%.3f\n", count, list_total_weight(head));
    for (int i = 0; i < count; i++) {
        node_t *found = list_find(head, keys[i]);
        if (found != NULL) {
            printf("key=%d name=%s weight=%.3f\n", found->key, found->name, found->weight);
        }
    }

    node_free(head);
    return EXIT_SUCCESS;
}
