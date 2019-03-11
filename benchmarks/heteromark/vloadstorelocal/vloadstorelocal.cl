__kernel void vlsl(                                                
   __global int* a,                                               
   __global int* c,                                               
   __local  int* a_tmp,
   const unsigned int count
   )
{
			int ltid = get_local_id(0);
			int gtid = get_global_id(0);
			
			uint group_id = get_group_id(0);

			int i;
			int j;
			int interim;
		
			if((gtid*16)<count)
			{
				for(i=0; i<16; i++)
				{
					a_tmp[ltid*16 + i] = a[gtid*16 + i];
				}	

                     		// wait until the whole block is filled
                     		barrier(CLK_LOCAL_MEM_FENCE);
		
				for(i=0; i<16; i++)
				{
				//		c[gtid*16 + i] = a_tmp[ltid*16 + i];
				//	c[gtid*16 + i] = 0;
//					interim = 0;
//					for(j=0; j<500; j++)
//					{
//						interim += (j / a_tmp[ltid*16 + i]);
//						interim *= j;
//						//c[gtid*16 + i] += (j / a_tmp[ltid*16 + i]);
//						//c[gtid*16 + i] *=j;
//						//c[gtid*16 + i] = a_tmp[ltid*16+i];
//					}
						c[gtid*16 + i] = a_tmp[ltid*16 + i];
//						c[gtid*16 + i] = interim;
				}

                     		// wait until the whole block is filled
                     		barrier(CLK_LOCAL_MEM_FENCE);
			}
}
