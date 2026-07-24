import torch
import torch.nn as nn

class SharedEncoderNet(nn.Module):
    def __init__(self, input_dim=7, num_agents=3):
        super(SharedEncoderNet, self).__init__()
        
        # Core trunk: Learns foundational system state
        self.shared_trunk = nn.Sequential(
            nn.Linear(input_dim, 64),
            nn.ReLU(),
            nn.Dropout(0.05),
            nn.Linear(64, 32),
            nn.ReLU()
        )
        
        # Head 1: Probability of Success (0.0 to 1.0)
        self.quality_head = nn.Sequential(
            nn.Linear(32, 16),
            nn.ReLU(),
            nn.Linear(16, num_agents),
            nn.Sigmoid()
        )
        
        # Head 2: Latency in seconds (Positive continuous)
        self.latency_head = nn.Sequential(
            nn.Linear(32, 16),
            nn.ReLU(),
            nn.Linear(16, num_agents),
            nn.Softplus()
        )
        
        # Head 3: Cost estimates (Positive continuous)
        self.cost_head = nn.Sequential(
            nn.Linear(32, 16),
            nn.ReLU(),
            nn.Linear(16, num_agents),
            nn.Softplus()
        )

    def forward(self, x):
        features = self.shared_trunk(x)
        q_pred = self.quality_head(features)
        l_pred = self.latency_head(features)
        c_pred = self.cost_head(features)
        return q_pred, l_pred, c_pred