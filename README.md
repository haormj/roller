## roller

### 概述

roller提供文件写入滚动，滚动支持按照文件大小和时间间隔，并支持对滚动的后的文件进行生命周期管理。

### 快速开始

见 example

### 功能介绍

- 文件轮换策略，两个策略任何是与的关系
    - 支持按照文件大小轮换
    - 支持按照时间间隔，比如每10分钟滚一个文件
- 文件保留策略（多个条件若指定了，则必须都满足）
    - 支持设置文件保留时间
    - 支持设置文件保留数量
    - 支持设置文件保留

### 使用说明

- 基于文件大小来轮换的，每次停止为了让这个日志能够被收集到，需要手动调用Rotate，然后在调用Close