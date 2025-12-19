run_8192:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 480 512 544 576 608 640 672 704 736; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_8192_small:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 480 512 544 576; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_8192_large:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 640 672 704 736; do \
		gt_file="data/simtune-example-8192-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-example-8192-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_rodinia_b+tree:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 1000 3000 5000; do \
		gt_file="data/rodinia-b+tree-128-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/rodinia-b+tree-128-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_simtune_ffmakernel:
	@printf "thread\ttruth\tsimulation\n"
	@for t in 10 20 30 40; do \
		gt_file="data/simtune-ffmakernel-1-32-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-ffmakernel-1-32-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_simtune_ffmakernel-large-large-thread128:
	@printf "#threadblock\ttruth\tsimulation\n"
	@for t in 600 1000 1400; do \
		gt_file="data/simtune-ffmakernel-large-large-$$t-128-10/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-ffmakernel-large-large-$$t-128-10); \
		printf "%s\t\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_simtune_ffmakernel-large-large-thread1024:
	@printf "#threadblock\ttruth\tsimulation\n"
	@for t in 200 600 1000; do \
		gt_file="data/simtune-ffmakernel-large-large-$$t-1024-1/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-ffmakernel-large-large-$$t-1024-1); \
		printf "%s\t\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_rodinia_kmeans_size384:
	@printf "#clusters\ttruth\tsimulation\n"
	@for clusters in 16 24 32; do \
		gt_file="data/rodinia-kmeans-size384-clusters$$clusters/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/rodinia-kmeans-size384-clusters$$clusters); \
		printf "%s\t\t%s\t%s\n" "$$clusters" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*

run_simtune-pc:
# 	@printf "L1\n"
# 	@printf "#size\ttruth\t\tsimulation\tsimulation_time(min)\n"
# 	@for exp in 9 10 11; do \
# 		size=$$((1<<exp)); \
# 		gt_file="data/simtune-pointerchasing-l1-size2^$$exp/gt"; \
# 		truth=$$(cat $$gt_file); \
# 		start=$$(python -c 'import time; print(time.time())'); \
# 		sim=$$(./nvidia -trace-dir data/simtune-pointerchasing-l1-size2^$$exp); \
# 		end=$$(python -c 'import time; print(time.time())'); \
# 		simf=$$(awk -v v="$$sim" 'BEGIN{printf "%.3E", v}'); \
# 		sim_time_min=$$(awk -v s="$$start" -v e="$$end" 'BEGIN{printf "%.2f", (e-s)/60 }'); \
# 		printf "%s\t%s\t%s\t%s\n" "$$size" "$$truth" "$$simf" "$$sim_time_min"; \
# 	done
# 	@printf "L2\n"
# 	@printf "#size\ttruth\t\tsimulation\tsimulation_time(min)\n"
# 	@for exp in 16 18 20; do \
# 		size=$$((1<<exp)); \
# 		gt_file="data/simtune-pointerchasing-l2-size2^$$exp/gt"; \
# 		truth=$$(cat $$gt_file); \
# 		start=$$(python -c 'import time; print(time.time())'); \
# 		sim=$$(./nvidia -trace-dir data/simtune-pointerchasing-l2-size2^$$exp); \
# 		end=$$(python -c 'import time; print(time.time())'); \
# 		simf=$$(awk -v v="$$sim" 'BEGIN{printf "%.3E", v}'); \
# 		sim_time_min=$$(awk -v s="$$start" -v e="$$end" 'BEGIN{printf "%.2f", (e-s)/60 }'); \
# 		printf "%s\t%s\t%s\t%s\n" "$$size" "$$truth" "$$simf" "$$sim_time_min"; \
# 	done
	@printf "DRAM\n"
	@printf "#size\ttruth\t\tsimulation\tsimulation_time(min)\n"
	@for exp in 24 26 28; do \
		size=$$((1<<exp)); \
		gt_file="data/simtune-pointerchasing-dram-size2^$$exp/gt"; \
		truth=$$(cat $$gt_file); \
		start=$$(python -c 'import time; print(time.time())'); \
		sim=$$(./nvidia -trace-dir data/simtune-pointerchasing-dram-size2^$$exp); \
		end=$$(python -c 'import time; print(time.time())'); \
		simf=$$(awk -v v="$$sim" 'BEGIN{printf "%.3E", v}'); \
		sim_time_min=$$(awk -v s="$$start" -v e="$$end" 'BEGIN{printf "%.2f", (e-s)/60 }'); \
		printf "%s\t%s\t%s\t%s\n" "$$size" "$$truth" "$$simf" "$$sim_time_min"; \
	done
	@rm akita_sim_d*