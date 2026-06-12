// huffman.cpp -- HIP benchmark for Rodinia Huffman Encoding
// Parallel histogram (GPU), codebook construction (CPU), parallel encoding (GPU).

#include "bench_common_hip.h"
#include <algorithm>

// Kernel: compute byte frequency histogram using atomicAdd
__global__ void histogram_kernel(const unsigned char *data,
                                 unsigned int *hist, int data_size) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= data_size) return;

    atomicAdd(&hist[data[tid]], 1u);
}

// Kernel: encode each byte using a codebook stored in global memory
__global__ void encode_kernel(const unsigned char *data,
                              const unsigned int *codebook_codes,
                              const unsigned int *codebook_lengths,
                              unsigned int *output_codes,
                              unsigned int *output_lengths,
                              int data_size) {
    int tid = hipBlockIdx_x * hipBlockDim_x + hipThreadIdx_x;
    if (tid >= data_size) return;

    unsigned char sym = data[tid];
    output_codes[tid] = codebook_codes[sym];
    output_lengths[tid] = codebook_lengths[sym];
}

// --- CPU Huffman codebook construction ---

struct HuffNode {
    unsigned int freq;
    int symbol;   // -1 for internal nodes
    int left, right;
};

// Build Huffman tree and extract codes
void build_codebook(const unsigned int *hist, int num_symbols,
                    unsigned int *codes, unsigned int *lengths) {
    // Initialize
    for (int i = 0; i < num_symbols; i++) {
        codes[i] = 0;
        lengths[i] = 0;
    }

    // Collect non-zero symbols
    std::vector<int> active;
    std::vector<HuffNode> nodes;
    nodes.reserve(2 * num_symbols);

    for (int i = 0; i < num_symbols; i++) {
        if (hist[i] > 0) {
            int idx = (int)nodes.size();
            HuffNode n;
            n.freq = hist[i];
            n.symbol = i;
            n.left = -1;
            n.right = -1;
            nodes.push_back(n);
            active.push_back(idx);
        }
    }

    if (active.empty()) return;

    // Special case: single symbol
    if (active.size() == 1) {
        codes[nodes[active[0]].symbol] = 0;
        lengths[nodes[active[0]].symbol] = 1;
        return;
    }

    // Build tree using simple O(n^2) method (fine for <=256 symbols)
    while (active.size() > 1) {
        // Find two nodes with smallest frequencies
        int min1 = 0, min2 = 1;
        if (nodes[active[min1]].freq > nodes[active[min2]].freq)
            std::swap(min1, min2);
        for (int i = 2; i < (int)active.size(); i++) {
            if (nodes[active[i]].freq < nodes[active[min1]].freq) {
                min2 = min1;
                min1 = i;
            } else if (nodes[active[i]].freq < nodes[active[min2]].freq) {
                min2 = i;
            }
        }

        int idx1 = active[min1];
        int idx2 = active[min2];

        HuffNode parent;
        parent.freq = nodes[idx1].freq + nodes[idx2].freq;
        parent.symbol = -1;
        parent.left = idx1;
        parent.right = idx2;
        int pidx = (int)nodes.size();
        nodes.push_back(parent);

        // Remove min1 and min2, add parent
        if (min1 > min2) std::swap(min1, min2);
        active.erase(active.begin() + min2);
        active.erase(active.begin() + min1);
        active.push_back(pidx);
    }

    // Traverse tree to extract codes using iterative DFS
    struct StackEntry {
        int node_idx;
        unsigned int code;
        unsigned int depth;
    };

    std::vector<StackEntry> stack;
    StackEntry root;
    root.node_idx = active[0];
    root.code = 0;
    root.depth = 0;
    stack.push_back(root);

    while (!stack.empty()) {
        StackEntry e = stack.back();
        stack.pop_back();

        const HuffNode &n = nodes[e.node_idx];
        if (n.symbol >= 0) {
            codes[n.symbol] = e.code;
            lengths[n.symbol] = (e.depth > 0) ? e.depth : 1;
        } else {
            if (n.left >= 0) {
                StackEntry left;
                left.node_idx = n.left;
                left.code = (e.code << 1);
                left.depth = e.depth + 1;
                stack.push_back(left);
            }
            if (n.right >= 0) {
                StackEntry right;
                right.node_idx = n.right;
                right.code = (e.code << 1) | 1;
                right.depth = e.depth + 1;
                stack.push_back(right);
            }
        }
    }
}

int main(int argc, char **argv) {
    int iterations = parseIterations(argc, argv);
    int data_size = parseIntParam(argc, argv, "--data_size", 1048576);
    int num_symbols = parseIntParam(argc, argv, "--num_symbols", 256);
    int block_size = parseIntParam(argc, argv, "--block_size", 256);
    int seed = parseIntParam(argc, argv, "--seed", 42);

    if (num_symbols > 256) num_symbols = 256;

    // Generate random byte data
    unsigned char *h_data =
        (unsigned char *)malloc(data_size * sizeof(unsigned char));
    srand(seed);
    for (int i = 0; i < data_size; i++) {
        h_data[i] = (unsigned char)(rand() % num_symbols);
    }

    // Device allocations
    unsigned char *d_data;
    unsigned int *d_hist, *d_codebook_codes, *d_codebook_lengths;
    unsigned int *d_output_codes, *d_output_lengths;
    HIP_CHECK(hipMalloc(&d_data, data_size * sizeof(unsigned char)));
    HIP_CHECK(hipMalloc(&d_hist, 256 * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_codebook_codes, 256 * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_codebook_lengths, 256 * sizeof(unsigned int)));
    HIP_CHECK(hipMalloc(&d_output_codes, data_size * sizeof(unsigned int)));
    HIP_CHECK(
        hipMalloc(&d_output_lengths, data_size * sizeof(unsigned int)));

    // Copy data to device
    HIP_CHECK(hipMemcpy(d_data, h_data,
                        data_size * sizeof(unsigned char),
                        hipMemcpyHostToDevice));

    // Host codebook arrays
    unsigned int h_hist[256];
    unsigned int h_codes[256];
    unsigned int h_lengths[256];

    dim3 block(block_size);
    dim3 grid((data_size + block_size - 1) / block_size);

    char problemSize[64];
    snprintf(problemSize, sizeof(problemSize), "%d", data_size);

    printCSVHeader();

    BenchResult r = runBenchmark("huffman", problemSize, iterations, [&]() {
        // Clear histogram
        HIP_CHECK(hipMemset(d_hist, 0, 256 * sizeof(unsigned int)));

        // 1. Compute histogram on GPU
        histogram_kernel<<<grid, block>>>(d_data, d_hist, data_size);
        HIP_CHECK(hipDeviceSynchronize());

        // Download histogram
        HIP_CHECK(hipMemcpy(h_hist, d_hist, 256 * sizeof(unsigned int),
                            hipMemcpyDeviceToHost));

        // 2. Build codebook on CPU
        build_codebook(h_hist, num_symbols, h_codes, h_lengths);

        // Upload codebook
        HIP_CHECK(hipMemcpy(d_codebook_codes, h_codes,
                            256 * sizeof(unsigned int),
                            hipMemcpyHostToDevice));
        HIP_CHECK(hipMemcpy(d_codebook_lengths, h_lengths,
                            256 * sizeof(unsigned int),
                            hipMemcpyHostToDevice));

        // 3. Encode on GPU
        encode_kernel<<<grid, block>>>(d_data, d_codebook_codes,
                                       d_codebook_lengths, d_output_codes,
                                       d_output_lengths, data_size);
        HIP_CHECK(hipDeviceSynchronize());
    });
    printCSVRow(r);

    // Cleanup
    HIP_CHECK(hipFree(d_data));
    HIP_CHECK(hipFree(d_hist));
    HIP_CHECK(hipFree(d_codebook_codes));
    HIP_CHECK(hipFree(d_codebook_lengths));
    HIP_CHECK(hipFree(d_output_codes));
    HIP_CHECK(hipFree(d_output_lengths));
    free(h_data);

    return 0;
}
