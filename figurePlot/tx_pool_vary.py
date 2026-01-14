import os
import pandas as pd
import matplotlib.pyplot as plt

directory = './expTest/result/pbft_shardNum=16'
output_dir = "./expTest"
file_name = 'TxPool_relay.png'

full_path = os.path.join(output_dir, file_name)

csv_files = [f for f in os.listdir(directory) if f.endswith('.csv')]

plt.figure(figsize=(10, 6))

legend_labels = []

x = 1

for csv_file in csv_files:
    file_path = os.path.join(directory, csv_file)
    
    df = pd.read_csv(file_path)
    
    block_height = df['Block Height']
    tx_pool_size = df['TxPool Size']
    
    parts = csv_file.split('_')
    if len(parts) > x:
        file_name = '_'.join(parts[:-x])
    else:
        file_name = csv_file
    
    plt.plot(block_height, tx_pool_size, label=file_name)
    
    legend_labels.append(file_name)

plt.xlabel('Block Height')
plt.ylabel('TxPool Size')
plt.title('TxPool Size vs Block Height')

plt.legend()

plt.grid(True)
plt.tight_layout()
plt.savefig(full_path)
plt.close()