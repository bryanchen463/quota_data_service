#!/usr/bin/env python3
"""
自动生成Python protobuf代码的脚本

使用方法:
python generate_proto.py
"""

import os
import subprocess
import sys

def main():
    # 获取当前脚本所在目录
    current_dir = os.path.dirname(os.path.abspath(__file__))
    proto_dir = os.path.join(current_dir, '..', 'proto')
    output_dir = os.path.join(current_dir, 'proto')
    
    # 创建输出目录
    os.makedirs(output_dir, exist_ok=True)
    
    # 检查proto文件是否存在
    proto_file = os.path.join(proto_dir, 'quota_service.proto')
    if not os.path.exists(proto_file):
        print(f"Error: Proto file not found: {proto_file}")
        sys.exit(1)
    
    # 生成Python代码
    cmd = [
        sys.executable, '-m', 'grpc_tools.protoc',
        f'-I{proto_dir}',
        f'--python_out={output_dir}',
        f'--grpc_python_out={output_dir}',
        proto_file
    ]
    
    print("Generating Python protobuf code...")
    print(f"Command: {' '.join(cmd)}")
    
    try:
        result = subprocess.run(cmd, capture_output=True, text=True, check=True)
        print("Successfully generated Python protobuf code!")
        print(f"Output directory: {output_dir}")
        
        # 列出生成的文件
        generated_files = [f for f in os.listdir(output_dir) if f.endswith('.py')]
        if generated_files:
            print("Generated files:")
            for f in generated_files:
                print(f"  - {f}")
        else:
            print("No Python files were generated.")
            
    except subprocess.CalledProcessError as e:
        print(f"Error generating protobuf code: {e}")
        print(f"stdout: {e.stdout}")
        print(f"stderr: {e.stderr}")
        sys.exit(1)
    except FileNotFoundError:
        print("Error: grpcio-tools not found. Please install it first:")
        print("pip install grpcio-tools")
        sys.exit(1)

if __name__ == '__main__':
    main() 