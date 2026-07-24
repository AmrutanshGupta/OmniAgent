import os
import time
import torch
import numpy as np
from datasets import load_dataset
from httpx import ConnectError


def load_dataset_with_retry(repo_id, split, streaming, max_retries=4, base_delay=3):
    """
    Hugging Face Hub connections drop intermittently on Windows
    (WinError 10054). Retry with exponential backoff before giving up.
    """
    for attempt in range(1, max_retries + 1):
        try:
            return load_dataset(repo_id, split=split, streaming=streaming)
        except ConnectError as e:
            if attempt == max_retries:
                raise
            wait = base_delay * (2 ** (attempt - 1))
            print(f"Connection dropped (attempt {attempt}/{max_retries}). "
                  f"Retrying in {wait}s...")
            time.sleep(wait)


def build_hybrid_telemetry_dataset():
    print("Streaming real human preference data from Chatbot Arena...")
    dataset = load_dataset_with_retry(
        "lmarena-ai/arena-human-preference-140k",
        split="train",
        streaming=True,
    )

    inputs, target_q, target_l, target_c = [], [], [], []

    print("Simulating infrastructure metrics...")
    for idx, row in enumerate(dataset.take(1000)):

        # full_conversation[0] is a turn dict: {"user": {"role":..., "content":[{"type":"text","text":...}]}, ...}
        try:
            prompt = row["full_conversation"][0]["user"]["content"][0]["text"]
        except (IndexError, KeyError, TypeError):
            prompt = ""
        prompt_tokens = row.get("conv_metadata", {}).get("token_counts", {}).get("user", len(prompt.split()))

        is_code = 1.0 if "code" in prompt.lower() else 0.0
        complexity_heuristic = (prompt_tokens * 0.05) + (is_code * 2.0)

        q0 = float(np.random.randint(0, 15))
        q1 = float(np.random.randint(0, 8))
        q2 = float(np.random.randint(0, 5))
        gpu_util = float(np.random.uniform(0.2, 0.95))

        winner = row["winner"]
        if winner == "model_a":
            true_q = [0.95, 0.60, 0.40]
        elif winner == "model_b":
            true_q = [0.95, 0.85, 0.50]
        else:
            true_q = [0.90, 0.75, 0.70]

        base_latencies = [1.2, 0.7, 0.2]
        true_l = [base_latencies[0] + (q0 * 0.1), base_latencies[1] + (q1 * 0.05), base_latencies[2] + (q2 * 0.01)]

        true_c = [prompt_tokens * 0.000005, prompt_tokens * 0.000002, prompt_tokens * 0.00000015]

        inputs.append([prompt_tokens, complexity_heuristic, is_code, q0, q1, q2, gpu_util])
        target_q.append(true_q)
        target_l.append(true_l)
        target_c.append(true_c)

    os.makedirs(os.path.join(os.path.dirname(__file__), "data"), exist_ok=True)

    payload = {
        "inputs": torch.tensor(inputs, dtype=torch.float32),
        "target_q": torch.tensor(target_q, dtype=torch.float32),
        "target_l": torch.tensor(target_l, dtype=torch.float32),
        "target_c": torch.tensor(target_c, dtype=torch.float32)
    }

    save_path = os.path.join(os.path.dirname(__file__), "data", "processed_training_set.pt")
    torch.save(payload, save_path)
    print(f"Data engine compilation verified. Saved {len(inputs)} records to {save_path}.")


if __name__ == "__main__":
    build_hybrid_telemetry_dataset()