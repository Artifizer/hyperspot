[THIS IS WORK IN PROGRESS]

# HyperSpot Server

A Powerful service written in Go for managing, interacting with, and evaluating large language models (LLMs) across local and cloud-based providers. It combines model orchestration, chat, benchmarking and intelligent files search into a unified control plane.


## Overview

The HyperSpot is a comprehensive server application designed to integrate and manage multiple LLM services. It provides secure HTTP API endpoints to:
- Manage local and remote LLM services (e.g., Ollama, LM Studio, Jan.ai, GPT4All, LocalAI, Cortex, OpenAI, Anthropic)
- Manage models for local LLM services (download, start, stop, delete)
- Query and retrieve model details in common formats
- Execute hardware and LLM benchmarks (e.g., HumanEval, MBPP)
- Support chat interactions (both synchronous and streaming)
- Index local file system and search using LLMs
