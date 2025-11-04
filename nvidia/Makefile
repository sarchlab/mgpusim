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
	@for t in 16 64 256 512 768 1024; do \
		gt_file="data/simtune-ffmakernel-4096-$$t/gt"; \
		truth=$$(cat $$gt_file); \
		sim=$$(./nvidia -trace-dir data/simtune-ffmakernel-4096-$$t); \
		printf "%s\t%s\t%s\n" "$$t" "$$truth" "$$sim"; \
	done
	@rm akita_sim_d*