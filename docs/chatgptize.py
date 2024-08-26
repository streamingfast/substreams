#!/usr/bin/env python3

import os
import glob

def read_markdown_files():
    combined_content = ""
    for root, _, files in os.walk('.'):
        for file in files:
            if file.endswith('.md'):
                file_path = os.path.join(root, file)
                with open(file_path, 'r', encoding='utf-8') as f:
                    content = f.read()
                    combined_content += f"\n;;;;;;; FILE BEGIN: {file_path} ;;;;;;;\n{content}\n;;;;;;; FILE END ;;;;;;;\n\n"
    return combined_content

def main():
    print(
        "Please rewrite the following markdown content according to the Google Documentation Style Guide and Diataxis principles. "
        "Make sure the structure is clear, concise, and follows best practices for user documentation.\n\n"
        "Use the same FILE BEGIN and FILE END delimitation as the original content.\n"
        "The SUMMARY.md is the table of contents and should point to files refered elsewhere. It is a special file that should look very similar to the input file.\n"
    )
    content = read_markdown_files()
    print(content)

if __name__ == "__main__":
    main()
