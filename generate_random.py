import random
import os

# 生成100个随机数（1到1000之间）
random_numbers = [random.randint(1, 1000) for _ in range(100)]

# 桌面路径
desktop_path = r"C:\Users\wheat\Desktop\随机数.txt"

# 写入文件
with open(desktop_path, 'w', encoding='utf-8') as f:
    for i, num in enumerate(random_numbers):
        if i == 99:  # 最后一个数字不换行
            f.write(str(num))
        else:
            f.write(f"{num}\n")

print(f"成功生成100个随机数到: {desktop_path}")
print(f"前5个随机数: {random_numbers[:5]}")
print(f"后5个随机数: {random_numbers[-5:]}")