package io.killedkenny.crossfuzz;

import java.lang.instrument.ClassFileTransformer;
import java.security.ProtectionDomain;
import java.util.HashSet;
import java.util.Set;
import org.objectweb.asm.ClassReader;
import org.objectweb.asm.ClassWriter;
import org.objectweb.asm.Opcodes;
import org.objectweb.asm.tree.AbstractInsnNode;
import org.objectweb.asm.tree.ClassNode;
import org.objectweb.asm.tree.InsnList;
import org.objectweb.asm.tree.JumpInsnNode;
import org.objectweb.asm.tree.LabelNode;
import org.objectweb.asm.tree.LdcInsnNode;
import org.objectweb.asm.tree.LookupSwitchInsnNode;
import org.objectweb.asm.tree.MethodInsnNode;
import org.objectweb.asm.tree.MethodNode;
import org.objectweb.asm.tree.TableSwitchInsnNode;
import org.objectweb.asm.tree.TryCatchBlockNode;

public class CoverageTransformer implements ClassFileTransformer {

    private static final String[] EXCLUDED = {
        "io/killedkenny/crossfuzz/", "java/", "javax/", "sun/", "com/sun/",
        "jdk/", "org/objectweb/asm/"
    };

    @Override
    public byte[] transform(ClassLoader loader, String className,
            Class<?> classBeingRedefined, ProtectionDomain pd, byte[] buf) {
        if (className == null) return null;
        for (String p : EXCLUDED) {
            if (className.startsWith(p)) return null;
        }
        try {
            ClassReader cr = new ClassReader(buf);
            ClassNode cn = new ClassNode();
            // SKIP_FRAMES: don't parse existing frames — leaves no FrameNodes in the tree
            // so insertions after LabelNodes are always placed before the first real insn.
            cr.accept(cn, ClassReader.SKIP_FRAMES);
            for (MethodNode mn : cn.methods) {
                instrumentMethod(className, mn);
            }
            // COMPUTE_FRAMES: recompute all stack map frames from scratch so the
            // inserted hit() calls don't invalidate existing frames. Resolving the
            // common superclass of two merged types must go through the *target's*
            // class loader — ASM's default uses the ASM library's own loader, which
            // cannot see application classes and would type merged frames as Object,
            // producing a VerifyError when the JVM loads the instrumented class.
            ClassWriter cw = new ClassWriter(ClassWriter.COMPUTE_FRAMES) {
                @Override
                protected String getCommonSuperClass(String type1, String type2) {
                    return commonSuperClass(type1, type2, loader);
                }
            };
            cn.accept(cw);
            return cw.toByteArray();
        } catch (Throwable t) {
            return null;
        }
    }

    /**
     * Resolves the common superclass of two internal type names by loading them
     * through the target application's class loader. Mirrors the algorithm of
     * ASM's default {@code ClassWriter.getCommonSuperClass}, which otherwise
     * resolves against the ASM library's own loader and cannot see application
     * classes — yielding frames typed too loosely (Object) and a VerifyError
     * when the JVM loads the instrumented class.
     *
     * <p>If a type cannot be resolved this throws; {@link #transform} catches it
     * and skips instrumentation for the whole class, so the uninstrumented class
     * still loads and runs — it only loses coverage.
     */
    private static String commonSuperClass(String type1, String type2,
            ClassLoader loader) {
        ClassLoader cl = (loader != null) ? loader : ClassLoader.getSystemClassLoader();
        Class<?> c, d;
        try {
            c = Class.forName(type1.replace('/', '.'), false, cl);
            d = Class.forName(type2.replace('/', '.'), false, cl);
        } catch (ClassNotFoundException e) {
            throw new RuntimeException(e);
        }
        if (c.isAssignableFrom(d)) return type1;
        if (d.isAssignableFrom(c)) return type2;
        if (c.isInterface() || d.isInterface()) {
            return "java/lang/Object";
        }
        do {
            c = c.getSuperclass();
        } while (!c.isAssignableFrom(d));
        return c.getName().replace('.', '/');
    }

    private void instrumentMethod(String cls, MethodNode mn) {
        // Collect branch target labels (basic block entries)
        Set<LabelNode> targets = new HashSet<>();
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof JumpInsnNode) {
                targets.add(((JumpInsnNode) n).label);
            } else if (n instanceof TableSwitchInsnNode) {
                TableSwitchInsnNode ts = (TableSwitchInsnNode) n;
                targets.add(ts.dflt);
                ts.labels.forEach(targets::add);
            } else if (n instanceof LookupSwitchInsnNode) {
                LookupSwitchInsnNode ls = (LookupSwitchInsnNode) n;
                targets.add(ls.dflt);
                ls.labels.forEach(targets::add);
            }
        }
        for (TryCatchBlockNode tcb : mn.tryCatchBlocks) {
            targets.add(tcb.handler);
        }

        // Inject at method entry (before first instruction)
        mn.instructions.insert(makeHit(cls, mn.name, 0));

        // Inject after each branch-target label
        int blockId = 1;
        for (AbstractInsnNode n : mn.instructions.toArray()) {
            if (n instanceof LabelNode && targets.contains(n)) {
                mn.instructions.insert(n, makeHit(cls, mn.name, blockId++));
            }
        }
    }

    private InsnList makeHit(String cls, String method, int blockId) {
        int idx = (cls + "_" + method + "_" + blockId).hashCode() & 0xFFFF;
        InsnList l = new InsnList();
        l.add(new LdcInsnNode(idx));
        l.add(new MethodInsnNode(Opcodes.INVOKESTATIC,
            "io/killedkenny/crossfuzz/CoverageRuntime", "hit", "(I)V", false));
        return l;
    }
}
