set(BENCHMARK_NAME pr_cl12)
set(SOURCE_FILES pr_cl12_benchmark.cc main.cc)
set(HEADER_FILES pr_cl12_benchmark.h)
set(EXTRA_FILES kernels.cl)

if(COMPILE_OPENCL12 AND OPENCL_INCLUDE_DIRS AND OPENCL_LIBRARIES )
  add_executable(${BENCHMARK_NAME} ${SOURCE_FILES} ${INCLUDE_FILES} ${EXTRA_FILES})

  target_link_libraries(${BENCHMARK_NAME} 
      pr cl_util time_measurement command_line_option benchmark 
      ${OPENCL_LIBRARIES})

  # Copy extra files to binary directory
  foreach(extra_file ${EXTRA_FILES})
    add_custom_command(
        TARGET ${BENCHMARK_NAME} POST_BUILD
        COMMAND ${CMAKE_COMMAND} -E copy_if_different
          ${CMAKE_CURRENT_SOURCE_DIR}/${extra_file}  ${CMAKE_CURRENT_BINARY_DIR}
        )
  endforeach(extra_file)
endif()
