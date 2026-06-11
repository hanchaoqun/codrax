cmake_minimum_required(VERSION 3.16)
project(fastlex_native CXX)
add_library(fastlex_native src/lexer.cpp)
target_include_directories(fastlex_native PUBLIC include)
